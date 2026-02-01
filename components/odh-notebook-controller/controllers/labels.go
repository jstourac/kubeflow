package controllers

// Label keys/values used across the controller for ownership, selection and cleanup.
// Keeping these centralized avoids subtle drift/typos across reconcilers, tests and e2e.
const (
	// Notebook identity labels (used for selecting related resources).
	NotebookNameLabelKey      = "notebook-name"
	NotebookNamespaceLabelKey = "notebook-namespace"

	// Controller ownership labels.
	ManagedByLabelKey   = "opendatahub.io/managed-by"
	ManagedByLabelValue = "workbenches"

	// Component identification labels.
	AppManagedByLabelKey   = "app.kubernetes.io/managed-by"
	AppManagedByLabelValue = "odh-notebook-controller"
	ComponentLabelKey      = "opendatahub.io/component"
	ComponentLabelValue    = "notebook-controller"

	// Runtime image identification label (OpenShift ImageStream).
	RuntimeImageLabelKey = "opendatahub.io/runtime-image"
)
