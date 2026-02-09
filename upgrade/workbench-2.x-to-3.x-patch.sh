# Variables
NAME="<VALUE>"
NAMESPACE="<VALUE>"

# Generate the patch string dynamically
PATCH=$(oc get notebook $NAME -n $NAMESPACE -o json | jq -c '
[
  {"op":"add","path":"/metadata/annotations/notebooks.opendatahub.io~1inject-auth","value":"true"},
  {"op":"remove","path":"/metadata/annotations/notebooks.opendatahub.io~1inject-oauth"},
  {"op":"remove","path":"/metadata/annotations/notebooks.opendatahub.io~1oauth-logout-url"},
  (
    .spec.template.spec.containers | to_entries[] |
    select(.value.name == "oauth-proxy") |
    {"op":"remove", "path": "/spec/template/spec/containers/\(.key)"}
  ),
  (
    .metadata.finalizers | to_entries[] |
    select(.value == "notebook-oauth-client-finalizer.opendatahub.io") |
    {"op":"remove", "path": "/metadata/finalizers/\(.key)"}
  ),
  (
    .spec.template.spec.volumes | to_entries[] |
    select(.value.name | IN("oauth-config", "oauth-client", "tls-certificates")) |
    {"op":"remove", "path": "/spec/template/spec/volumes/\(.key)"}
  )
] | sort_by(.path) | reverse')

# Execute the patch delete the StatefulSet for the workbench to workaround problem with kueue webhook sync
# https://issues.redhat.com/browse/RHOAIENG-49007
# The workbench has to be stopped or if it was running, it has to be stopped and started.
oc patch notebook $NAME -n $NAMESPACE --type='json' -p="$PATCH" && oc delete statefulset -n $NAMESPACE $NAME
