/*
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	nbv1 "github.com/kubeflow/kubeflow/components/notebook-controller/api/v1"
	"github.com/opendatahub-io/operator-chaos/pkg/sdk"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func initChaosReconciler(faultCfg *sdk.FaultConfig) *OpenshiftNotebookReconciler {
	chaosClient := sdk.NewChaosClient(cli, faultCfg)
	return &OpenshiftNotebookReconciler{
		Client:        chaosClient,
		Scheme:        cli.Scheme(),
		Log:           ctrl.Log.WithName("chaos-test"),
		Namespace:     odhNotebookControllerTestNamespace,
		Config:        cfg,
		EventRecorder: record.NewFakeRecorder(10),
		MLflowEnabled: false,
		GatewayURL:    "",
	}
}

var _ = Describe("ODH Notebook controller chaos resilience", func() {

	ctx := context.Background()

	var (
		chaosNamespace    *corev1.Namespace
		typeNamespaceName types.NamespacedName
	)

	createChaosNotebook := func(name string, faultCfg *sdk.FaultConfig) *OpenshiftNotebookReconciler {
		chaosNamespace = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		}
		typeNamespaceName = types.NamespacedName{Name: name, Namespace: name}

		err := cli.Create(ctx, chaosNamespace)
		Expect(err).NotTo(HaveOccurred())

		notebook := &nbv1.Notebook{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: name,
			},
			Spec: nbv1.NotebookSpec{
				Template: nbv1.NotebookTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{
							Name:  name,
							Image: "registry.redhat.io/ubi9/ubi:latest",
						}},
					},
				},
			},
		}

		err = cli.Create(ctx, notebook)
		Expect(err).NotTo(HaveOccurred())

		return initChaosReconciler(faultCfg)
	}

	AfterEach(func() {
		if chaosNamespace != nil {
			notebook := &nbv1.Notebook{}
			err := cli.Get(ctx, typeNamespaceName, notebook)
			if err == nil {
				notebook.Finalizers = nil
				_ = cli.Update(ctx, notebook)
				_ = cli.Delete(ctx, notebook)
			}
			_ = cli.Delete(ctx, chaosNamespace)
			chaosNamespace = nil
		}
	})

	It("should handle Get errors with requeue", func() {
		faultCfg := sdk.NewFaultConfig(map[sdk.Operation]sdk.FaultSpec{
			sdk.OpGet: {ErrorRate: 1.0, Error: "chaos: connection refused"},
		})

		reconciler := createChaosNotebook("chaos-get-errors", faultCfg)

		_, err := reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: typeNamespaceName,
		})

		Expect(err).To(HaveOccurred(), "expected a chaos error on Get")
		var chaosErr *sdk.ChaosError
		Expect(errors.As(err, &chaosErr)).To(BeTrue(), "error should be a ChaosError")
		Expect(chaosErr.Operation).To(Equal(sdk.OpGet))
	})

	It("should converge after transient Get errors clear", func() {
		faultCfg := sdk.NewFaultConfig(map[sdk.Operation]sdk.FaultSpec{
			sdk.OpGet: {ErrorRate: 1.0, Error: "chaos: transient connection refused"},
		})

		reconciler := createChaosNotebook("chaos-get-transient", faultCfg)

		By("Verifying reconciler fails while faults are active")
		_, err := reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: typeNamespaceName,
		})
		Expect(err).To(HaveOccurred())

		By("Clearing faults and verifying convergence")
		faultCfg.Deactivate()

		Eventually(func() error {
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespaceName,
			})
			return err
		}, 30*time.Second, 500*time.Millisecond).Should(Succeed(),
			"reconciler should converge once Get errors stop")
	})

	It("should handle Create failures and report errors", func() {
		faultCfg := sdk.NewFaultConfig(map[sdk.Operation]sdk.FaultSpec{
			sdk.OpCreate: {ErrorRate: 1.0, Error: "chaos: quota exceeded"},
		})

		reconciler := createChaosNotebook("chaos-create-fail", faultCfg)

		// The reconciler may need multiple passes before hitting a Create call
		// (the first pass adds finalizers via Update and returns Requeue).
		var lastErr error
		Eventually(func() bool {
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespaceName,
			})
			if err != nil {
				lastErr = err
				return true
			}
			return false
		}, 10*time.Second, 200*time.Millisecond).Should(BeTrue(),
			"reconciler should eventually hit a Create error")

		var chaosErr *sdk.ChaosError
		Expect(errors.As(lastErr, &chaosErr)).To(BeTrue(), "error should be a ChaosError")
		Expect(chaosErr.Operation).To(Equal(sdk.OpCreate))
	})

	It("should converge after transient Create failures clear", func() {
		faultCfg := sdk.NewFaultConfig(map[sdk.Operation]sdk.FaultSpec{
			sdk.OpCreate: {ErrorRate: 1.0, Error: "chaos: quota exceeded"},
		})

		reconciler := createChaosNotebook("chaos-create-transient", faultCfg)

		By("Verifying reconciler hits Create errors while faults are active")
		Eventually(func() bool {
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespaceName,
			})
			if err != nil {
				var chaosErr *sdk.ChaosError
				return errors.As(err, &chaosErr) && chaosErr.Operation == sdk.OpCreate
			}
			return false
		}, 10*time.Second, 200*time.Millisecond).Should(BeTrue(),
			"reconciler should hit a Create error")

		By("Clearing faults and verifying convergence")
		faultCfg.Deactivate()

		Eventually(func() error {
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespaceName,
			})
			return err
		}, 30*time.Second, 500*time.Millisecond).Should(Succeed(),
			"reconciler should converge once Create failures stop")
	})

	It("should remain converged when Update faults are present but no drift exists", func() {
		reconciler := createChaosNotebook("chaos-update-no-drift", nil)

		By("Running initial clean reconcile to create resources")
		Eventually(func() error {
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespaceName,
			})
			return err
		}, 30*time.Second, 500*time.Millisecond).Should(Succeed())

		By("Injecting Update faults and verifying reconciler stays healthy")
		updateFaultCfg := sdk.NewFaultConfig(map[sdk.Operation]sdk.FaultSpec{
			sdk.OpUpdate: {ErrorRate: 1.0, Error: "chaos: the object has been modified"},
		})
		reconciler.Client = sdk.NewChaosClient(cli, updateFaultCfg)

		_, err := reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: typeNamespaceName,
		})
		Expect(err).NotTo(HaveOccurred(),
			"reconciler should succeed when state is converged and no updates are needed")
	})

	It("should tolerate intermittent API errors and eventually converge", func() {
		faultCfg := sdk.NewFaultConfig(map[sdk.Operation]sdk.FaultSpec{
			sdk.OpGet:    {ErrorRate: 0.15, Error: "chaos: intermittent timeout"},
			sdk.OpCreate: {ErrorRate: 0.15, Error: "chaos: intermittent quota exceeded"},
		})

		reconciler := createChaosNotebook("chaos-intermittent", faultCfg)

		Eventually(func() error {
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespaceName,
			})
			return err
		}, 30*time.Second, 200*time.Millisecond).Should(Succeed(),
			"reconciler should eventually converge despite intermittent errors")
	})
})
