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
	"testing"

	nbv1 "github.com/kubeflow/kubeflow/components/notebook-controller/api/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestValidateOAuthProxyResources(t *testing.T) {
	tests := []struct {
		name          string
		notebook      *nbv1.Notebook
		expectError   bool
		errorContains string
	}{
		{
			name: "valid annotations",
			notebook: &nbv1.Notebook{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						AnnotationOAuthProxyCPURequest:    "200m",
						AnnotationOAuthProxyMemoryRequest: "128Mi",
						AnnotationOAuthProxyCPULimit:      "500m",
						AnnotationOAuthProxyMemoryLimit:   "256Mi",
					},
				},
			},
			expectError: false,
		},
		{
			name: "no annotations",
			notebook: &nbv1.Notebook{
				ObjectMeta: metav1.ObjectMeta{},
			},
			expectError: false,
		},
		{
			name: "invalid CPU request",
			notebook: &nbv1.Notebook{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						AnnotationOAuthProxyCPURequest: "invalid-cpu",
					},
				},
			},
			expectError:   true,
			errorContains: "invalid CPU request value",
		},
		{
			name: "invalid memory request",
			notebook: &nbv1.Notebook{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						AnnotationOAuthProxyMemoryRequest: "invalid-memory",
					},
				},
			},
			expectError:   true,
			errorContains: "invalid memory request value",
		},
		{
			name: "CPU request greater than limit",
			notebook: &nbv1.Notebook{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						AnnotationOAuthProxyCPURequest: "500m",
						AnnotationOAuthProxyCPULimit:   "200m",
					},
				},
			},
			expectError:   true,
			errorContains: "CPU request (500m) cannot be greater than CPU limit (200m)",
		},
		{
			name: "memory request greater than limit",
			notebook: &nbv1.Notebook{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						AnnotationOAuthProxyMemoryRequest: "512Mi",
						AnnotationOAuthProxyMemoryLimit:   "256Mi",
					},
				},
			},
			expectError:   true,
			errorContains: "memory request (512Mi) cannot be greater than memory limit (256Mi)",
		},
		{
			name: "equal requests and limits",
			notebook: &nbv1.Notebook{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						AnnotationOAuthProxyCPURequest:    "200m",
						AnnotationOAuthProxyCPULimit:      "200m",
						AnnotationOAuthProxyMemoryRequest: "128Mi",
						AnnotationOAuthProxyMemoryLimit:   "128Mi",
					},
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateOAuthProxyResources(tt.notebook)
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				} else if tt.errorContains != "" && !containsString(err.Error(), tt.errorContains) {
					t.Errorf("expected error to contain '%s', got '%s'", tt.errorContains, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("expected no error but got: %v", err)
				}
			}
		})
	}
}

func TestGetOAuthProxyResourceRequirements(t *testing.T) {
	tests := []struct {
		name             string
		notebook         *nbv1.Notebook
		expectedCPUReq   string
		expectedMemReq   string
		expectedCPULimit string
		expectedMemLimit string
	}{
		{
			name: "no annotations - use defaults",
			notebook: &nbv1.Notebook{
				ObjectMeta: metav1.ObjectMeta{},
			},
			expectedCPUReq:   "100m",
			expectedMemReq:   "64Mi",
			expectedCPULimit: "100m",
			expectedMemLimit: "64Mi",
		},
		{
			name: "all annotations present",
			notebook: &nbv1.Notebook{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						AnnotationOAuthProxyCPURequest:    "200m",
						AnnotationOAuthProxyMemoryRequest: "128Mi",
						AnnotationOAuthProxyCPULimit:      "500m",
						AnnotationOAuthProxyMemoryLimit:   "256Mi",
					},
				},
			},
			expectedCPUReq:   "200m",
			expectedMemReq:   "128Mi",
			expectedCPULimit: "500m",
			expectedMemLimit: "256Mi",
		},
		{
			name: "partial annotations",
			notebook: &nbv1.Notebook{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						AnnotationOAuthProxyCPURequest:  "300m",
						AnnotationOAuthProxyMemoryLimit: "512Mi",
					},
				},
			},
			expectedCPUReq:   "300m",
			expectedMemReq:   "64Mi", // default
			expectedCPULimit: "100m", // default
			expectedMemLimit: "512Mi",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resources := getOAuthProxyResourceRequirements(tt.notebook)

			// Check CPU request
			if resources.Requests.Cpu().String() != tt.expectedCPUReq {
				t.Errorf("expected CPU request %s, got %s", tt.expectedCPUReq, resources.Requests.Cpu().String())
			}

			// Check memory request
			if resources.Requests.Memory().String() != tt.expectedMemReq {
				t.Errorf("expected memory request %s, got %s", tt.expectedMemReq, resources.Requests.Memory().String())
			}

			// Check CPU limit
			if resources.Limits.Cpu().String() != tt.expectedCPULimit {
				t.Errorf("expected CPU limit %s, got %s", tt.expectedCPULimit, resources.Limits.Cpu().String())
			}

			// Check memory limit
			if resources.Limits.Memory().String() != tt.expectedMemLimit {
				t.Errorf("expected memory limit %s, got %s", tt.expectedMemLimit, resources.Limits.Memory().String())
			}
		})
	}
}

func TestParseResourceQuantity(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expectNil bool
		expectErr bool
	}{
		{
			name:      "empty string returns nil",
			input:     "",
			expectNil: true,
			expectErr: false,
		},
		{
			name:      "valid CPU value",
			input:     "100m",
			expectNil: false,
			expectErr: false,
		},
		{
			name:      "valid memory value",
			input:     "64Mi",
			expectNil: false,
			expectErr: false,
		},
		{
			name:      "invalid value",
			input:     "invalid",
			expectNil: false,
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseResourceQuantity(tt.input)

			if tt.expectErr {
				if err == nil {
					t.Errorf("expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("expected no error but got: %v", err)
				}
			}

			if tt.expectNil {
				if result != nil {
					t.Errorf("expected nil but got: %v", result)
				}
			} else if !tt.expectErr {
				if result == nil {
					t.Errorf("expected non-nil result but got nil")
				} else if result.String() != tt.input {
					t.Errorf("expected result %s, got %s", tt.input, result.String())
				}
			}
		})
	}
}

func TestInjectOAuthProxyWithResourceValidation(t *testing.T) {
	tests := []struct {
		name        string
		notebook    *nbv1.Notebook
		expectError bool
	}{
		{
			name: "valid notebook with custom resources",
			notebook: &nbv1.Notebook{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-notebook",
					Namespace: "default",
					Annotations: map[string]string{
						AnnotationOAuthProxyCPURequest:    "200m",
						AnnotationOAuthProxyMemoryRequest: "128Mi",
						AnnotationOAuthProxyCPULimit:      "400m",
						AnnotationOAuthProxyMemoryLimit:   "256Mi",
					},
				},
				Spec: nbv1.NotebookSpec{
					Template: nbv1.NotebookTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "test-notebook",
									Image: "test-image",
								},
							},
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "invalid notebook - request > limit",
			notebook: &nbv1.Notebook{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-notebook",
					Namespace: "default",
					Annotations: map[string]string{
						AnnotationOAuthProxyCPURequest: "500m",
						AnnotationOAuthProxyCPULimit:   "200m",
					},
				},
				Spec: nbv1.NotebookSpec{
					Template: nbv1.NotebookTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "test-notebook",
									Image: "test-image",
								},
							},
						},
					},
				},
			},
			expectError: true,
		},
	}

	oauth := OAuthConfig{
		ProxyImage: "test-oauth-image",
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := InjectOAuthProxy(tt.notebook, oauth)
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("expected no error but got: %v", err)
				}

				// Verify that oauth-proxy container was added with correct resources
				var oauthContainer *corev1.Container
				for _, container := range tt.notebook.Spec.Template.Spec.Containers {
					if container.Name == "oauth-proxy" {
						oauthContainer = &container
						break
					}
				}

				if oauthContainer == nil {
					t.Errorf("oauth-proxy container not found")
					return
				}

				// Verify that custom resources were applied if annotations were present
				annotations := tt.notebook.GetAnnotations()
				if annotations != nil {
					if cpuReq, exists := annotations[AnnotationOAuthProxyCPURequest]; exists {
						if oauthContainer.Resources.Requests.Cpu().String() != cpuReq {
							t.Errorf("expected CPU request %s, got %s", cpuReq, oauthContainer.Resources.Requests.Cpu().String())
						}
					}
				}
			}
		})
	}
}

// Helper function to check if a string contains a substring
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (len(substr) == 0 || s[len(s)-len(substr):] == substr || s[:len(substr)] == substr || containsString(s[1:], substr))
}
