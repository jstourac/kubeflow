#!/bin/bash

NAME="<VALUE>"
NAMESPACE="<VALUE>"

echo "=========================================================="
echo " Starting cleanup for Notebook: $NAME"
echo " Target Namespace:              $NAMESPACE"
echo "=========================================================="

echo "[1/3] Removing Route and Services for notebook '$NAME' in '$NAMESPACE'..."
oc delete route "$NAME" -n "$NAMESPACE" --ignore-not-found
oc delete service "$NAME" -n "$NAMESPACE" --ignore-not-found
oc delete service "${NAME}-tls" -n "$NAMESPACE" --ignore-not-found

echo "[2/3] Removing Secrets for notebook '$NAME' in '$NAMESPACE'..."
oc delete secret "${NAME}-oauth-client" -n "$NAMESPACE" --ignore-not-found
oc delete secret "${NAME}-oauth-config" -n "$NAMESPACE" --ignore-not-found
oc delete secret "${NAME}-tls" -n "$NAMESPACE" --ignore-not-found

echo "[3/3] Removing OAuthClient: ${NAME}-${NAMESPACE}-oauth-client"
oc delete oauthclient "${NAME}-${NAMESPACE}-oauth-client" --ignore-not-found

echo "=========================================================="
echo " Cleanup complete for '$NAME' in '$NAMESPACE'."
echo "=========================================================="
