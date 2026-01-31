package controllers

import (
	"testing"

	nbv1 "github.com/kubeflow/kubeflow/components/notebook-controller/api/v1"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestApplyUpgradeMigration_RunningNotebook_NoDisruption(t *testing.T) {
	nb := &nbv1.Notebook{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nb",
			Namespace: "ns",
			Finalizers: []string{
				NotebookOAuthClientFinalizer,
			},
		},
		Spec: nbv1.NotebookSpec{
			Template: nbv1.NotebookTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "nb", Image: "quay.io/acme/workbench:latest"},
						{Name: "oauth-proxy", Image: "quay.io/openshift/oauth-proxy:latest"},
					},
				},
			},
		},
	}

	changed := ApplyUpgradeMigration(admissionv1.Update, nb)
	if !changed {
		t.Fatalf("expected ApplyUpgradeMigration to report change")
	}
	if nb.Annotations == nil || nb.Annotations[AnnotationUpgradePending] == "" {
		t.Fatalf("expected %q annotation to be set", AnnotationUpgradePending)
	}
	if _, ok := nb.Annotations[AnnotationInjectAuth]; ok {
		t.Fatalf("did not expect %q to be set while notebook is running", AnnotationInjectAuth)
	}
	for _, c := range nb.Spec.Template.Spec.Containers {
		if c.Name == "oauth-proxy" {
			return // ok: must keep while running
		}
	}
	t.Fatalf("expected oauth-proxy sidecar to remain while notebook is running")
}

func TestApplyUpgradeMigration_StoppedNotebook_MigratesToInjectAuth(t *testing.T) {
	nb := &nbv1.Notebook{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nb",
			Namespace: "ns",
			Annotations: map[string]string{
				"kubeflow-resource-stopped": "true",
			},
			Finalizers: []string{
				NotebookOAuthClientFinalizer,
			},
		},
		Spec: nbv1.NotebookSpec{
			Template: nbv1.NotebookTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "nb", Image: "quay.io/acme/workbench:latest"},
						{Name: "oauth-proxy", Image: "quay.io/openshift/oauth-proxy:latest"},
					},
				},
			},
		},
	}

	changed := ApplyUpgradeMigration(admissionv1.Update, nb)
	if !changed {
		t.Fatalf("expected ApplyUpgradeMigration to report change")
	}
	if nb.Annotations == nil || nb.Annotations[AnnotationInjectAuth] != "true" {
		t.Fatalf("expected %q to be set to \"true\"", AnnotationInjectAuth)
	}
	if _, ok := nb.Annotations[AnnotationUpgradePending]; ok {
		t.Fatalf("did not expect %q to be set after migration", AnnotationUpgradePending)
	}
	for _, c := range nb.Spec.Template.Spec.Containers {
		if c.Name == "oauth-proxy" {
			t.Fatalf("expected oauth-proxy sidecar to be removed when notebook is stopped")
		}
	}
}
