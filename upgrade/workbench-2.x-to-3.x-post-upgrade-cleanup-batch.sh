#!/bin/bash

# Identify all notebooks that need cleaning
NOTEBOOKS=$(oc get notebooks --all-namespaces -o custom-columns=NAME:.metadata.name,NS:.metadata.namespace --no-headers)

echo "$NOTEBOOKS" | while read NAME NAMESPACE; do
    echo "--- Cleaning up secondary resources for: $NAME in $NAMESPACE ---"

    # Delete Routes and Services by name pattern
    oc delete route "$NAME" -n "$NAMESPACE" --ignore-not-found
    oc delete service "$NAME" "${NAME}-tls" -n "$NAMESPACE" --ignore-not-found

    # Delete Secrets by name pattern
    oc delete secret "${NAME}-oauth-client" "${NAME}-oauth-config" "${NAME}-tls" -n "$NAMESPACE" --ignore-not-found

    # Delete Cluster-scoped OAuthClient
    oc delete oauthclient "${NAME}-${NAMESPACE}-oauth-client" --ignore-not-found
done
