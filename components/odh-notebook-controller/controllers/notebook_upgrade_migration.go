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
	"os"
	"strconv"
	"strings"

	nbv1 "github.com/kubeflow/kubeflow/components/notebook-controller/api/v1"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// Legacy OAuth annotations used by workbenches in 2.x.
	// These are kept here so we can safely migrate users during 2.x -> 3.x upgrades.
	AnnotationInjectOAuthLegacy    = "notebooks.opendatahub.io/inject-oauth"
	AnnotationOAuthLogoutURLLegacy = "notebooks.opendatahub.io/oauth-logout-url"

	// A non-disruptive signal to users/UX that the Notebook needs a stop/start cycle
	// to complete upgrade-related migration.
	AnnotationUpgradePending = "notebooks.opendatahub.io/upgrade-pending"

	// Value used for AnnotationUpgradePending
	UpgradePendingReasonOAuthToInjectAuth = "stop/start required to migrate legacy oauth auth to inject-auth"

	// Legacy OAuth proxy sidecar container name commonly used in 2.x workbenches.
	LegacyOAuthProxyContainerName = "oauth-proxy"
)

func autoMigrateOnUpgradeEnabled() bool {
	// Default to enabled. If needed, operators can disable via env var.
	raw, ok := os.LookupEnv("AUTO_MIGRATE_LEGACY_OAUTH")
	if !ok || strings.TrimSpace(raw) == "" {
		return true
	}
	val, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil {
		// Fail-safe: keep enabled unless explicitly disabled
		return true
	}
	return val
}

func notebookSafeToMutateAuth(reqOp admissionv1.Operation, meta metav1.ObjectMeta) bool {
	if reqOp == admissionv1.Create {
		return true
	}
	// When the user stops a workbench, this annotation is set and replicas go to 0.
	// Mutating the pod template at that point is safe (no involuntary restart).
	if metav1.HasAnnotation(meta, "kubeflow-resource-stopped") {
		return true
	}
	// Explicit restart operation is also a user-triggered lifecycle event.
	if metav1.HasAnnotation(meta, "notebooks.opendatahub.io/notebook-restart") {
		return true
	}
	return false
}

func hasLegacyOAuthAnnotations(meta metav1.ObjectMeta) bool {
	if meta.Annotations == nil {
		return false
	}
	if v, ok := meta.Annotations[AnnotationInjectOAuthLegacy]; ok && strings.TrimSpace(v) != "" {
		return true
	}
	if v, ok := meta.Annotations[AnnotationOAuthLogoutURLLegacy]; ok && strings.TrimSpace(v) != "" {
		return true
	}
	return false
}

func hasLegacyOAuthProxySidecar(nb *nbv1.Notebook) bool {
	for _, c := range nb.Spec.Template.Spec.Containers {
		if c.Name == LegacyOAuthProxyContainerName {
			return true
		}
		if strings.Contains(strings.ToLower(c.Image), "oauth-proxy") && c.Name != nb.Name {
			return true
		}
	}
	return false
}

func removeLegacyOAuthProxySidecar(nb *nbv1.Notebook) {
	containers := nb.Spec.Template.Spec.Containers
	if len(containers) == 0 {
		return
	}

	filtered := make([]corev1.Container, 0, len(containers))
	for _, c := range containers {
		if c.Name == LegacyOAuthProxyContainerName {
			continue
		}
		if strings.Contains(strings.ToLower(c.Image), "oauth-proxy") && c.Name != nb.Name {
			continue
		}
		filtered = append(filtered, c)
	}
	nb.Spec.Template.Spec.Containers = filtered
}

func hasFinalizer(nb *nbv1.Notebook, finalizer string) bool {
	for _, f := range nb.Finalizers {
		if f == finalizer {
			return true
		}
	}
	return false
}

// ApplyUpgradeMigration mutates the Notebook CR to the 3.x authentication model in a way that:
// - does NOT restart running workbenches;
// - applies changes on the next user-triggered stop/restart/start lifecycle operation.
//
// It returns true if it mutated the Notebook.
func ApplyUpgradeMigration(reqOp admissionv1.Operation, nb *nbv1.Notebook) bool {
	if !autoMigrateOnUpgradeEnabled() {
		return false
	}

	// Only attempt migration when the notebook looks like a 2.x OAuth-based workbench
	// and it isn't already using inject-auth.
	alreadyInjectAuth := KubeRbacProxyInjectionIsEnabled(nb.ObjectMeta)
	legacyAuthDetected := hasLegacyOAuthAnnotations(nb.ObjectMeta) || hasLegacyOAuthProxySidecar(nb) || hasFinalizer(nb, NotebookOAuthClientFinalizer)
	if alreadyInjectAuth || !legacyAuthDetected {
		// Clear pending signal if previously set and no longer needed.
		if nb.Annotations != nil {
			delete(nb.Annotations, AnnotationUpgradePending)
		}
		return false
	}

	// If notebook is currently running, do not flip auth mode yet; just signal that
	// a stop/start is required.
	if !notebookSafeToMutateAuth(reqOp, nb.ObjectMeta) {
		if nb.Annotations == nil {
			nb.Annotations = map[string]string{}
		}
		nb.Annotations[AnnotationUpgradePending] = UpgradePendingReasonOAuthToInjectAuth
		return true
	}

	// Safe moment: user is creating, stopping, or explicitly restarting.
	if nb.Annotations == nil {
		nb.Annotations = map[string]string{}
	}
	delete(nb.Annotations, AnnotationUpgradePending)

	// Switch to the 3.x auth model.
	nb.Annotations[AnnotationInjectAuth] = "true"

	// Remove legacy OAuth-specific annotations to avoid confusion.
	delete(nb.Annotations, AnnotationInjectOAuthLegacy)
	delete(nb.Annotations, AnnotationOAuthLogoutURLLegacy)

	// Remove legacy oauth-proxy sidecar if present. kube-rbac-proxy injection will
	// be handled by the existing webhook logic once inject-auth is set.
	//
	// Important: we only do this in safe lifecycle moments to avoid involuntary restarts.
	removeLegacyOAuthProxySidecar(nb)

	// We intentionally do NOT delete the OAuthClient here:
	// - removing it while a workbench is running could disrupt existing sessions.
	// - leaving it for later cleanup is safe; deletion can be done on explicit admin action.
	return true
}
