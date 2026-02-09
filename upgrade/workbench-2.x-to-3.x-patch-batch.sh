#!/bin/bash

# Optional: Filter by namespace or use --all-namespaces
NAMESPACE_SELECTOR="--all-namespaces"

# Get all notebook names and their namespaces
oc get notebooks $NAMESPACE_SELECTOR -o custom-columns=NAME:.metadata.name,NS:.metadata.namespace --no-headers | while read NAME NAMESPACE; do

    echo "Processing Notebook: $NAME in Namespace: $NAMESPACE"

    # Generate the patch string
    PATCH=$(oc get notebook "$NAME" -n "$NAMESPACE" -o json | jq -c '
    [
      {"op":"add","path":"/metadata/annotations/notebooks.opendatahub.io~1inject-auth","value":"true"},
      {"op":"remove","path":"/metadata/annotations/notebooks.opendatahub.io~1inject-oauth"},
      {"op":"remove","path":"/metadata/annotations/notebooks.opendatahub.io~1oauth-logout-url"},
      ( .spec.template.spec.containers | to_entries[] | select(.value.name == "oauth-proxy") | {"op":"remove", "path": "/spec/template/spec/containers/\(.key)"} ),
      ( .metadata.finalizers // [] | to_entries[] | select(.value == "notebook-oauth-client-finalizer.opendatahub.io") | {"op":"remove", "path": "/metadata/finalizers/\(.key)"} ),
      ( .spec.template.spec.volumes // [] | to_entries[] | select(.value.name | IN("oauth-config", "oauth-client", "tls-certificates")) | {"op":"remove", "path": "/spec/template/spec/volumes/\(.key)"} )
    ] | sort_by(.path) | reverse')

    # Execute the patch only if PATCH is not empty/null
    if [ "$PATCH" != "[]" ] && [ -n "$PATCH" ]; then
        # Execute the patch delete the StatefulSet for the workbench to workaround problem with kueue webhook sync
        # https://issues.redhat.com/browse/RHOAIENG-49007
        # The workbench has to be stopped or if it was running, it has to be stopped and started.
        oc patch notebook "$NAME" -n "$NAMESPACE" --type='json' -p="$PATCH" && oc delete statefulset -n "$NAMESPACE" "$NAME"
    fi
done
