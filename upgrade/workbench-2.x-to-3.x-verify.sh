#!/bin/bash
	# Quick verification script for notebook migration
	NAME="pre-upgrade-unmanaged"
	NAMESPACE="a-kueue"

	echo "=== Checking Notebook: $NAME ==="

	# Check inject-auth annotation (should be "true")
	AUTH=$(oc get notebook $NAME -n $NAMESPACE -o jsonpath='{.metadata.annotations.notebooks\.opendatahub\.io/inject-auth}' 2>/dev/null)
	if [ "$AUTH" == "true" ]; then
	  echo "✓ PASS: inject-auth annotation is set to 'true'"
	else
	  echo "✗ FAIL: inject-auth annotation missing or incorrect (found: '$AUTH')"
	fi

	# Check inject-oauth annotation (should NOT exist)
	OAUTH=$(oc get notebook $NAME -n $NAMESPACE -o jsonpath='{.metadata.annotations.notebooks\.opendatahub\.io/inject-oauth}' 2>/dev/null)
	if [ -z "$OAUTH" ]; then
	  echo "✓ PASS: Legacy inject-oauth annotation removed"
	else
	  echo "✗ FAIL: Legacy inject-oauth annotation still exists: '$OAUTH'"
	fi

	# Check sidecar containers
	CONTAINERS=$(oc get notebook $NAME -n $NAMESPACE -o jsonpath='{.spec.template.spec.containers[*].name}' 2>/dev/null)
	if echo "$CONTAINERS" | grep -q "kube-rbac-proxy"; then
	  echo "✓ PASS: kube-rbac-proxy sidecar container present (RHOAI 3.x)"
	else
	  echo "✗ FAIL: kube-rbac-proxy sidecar container missing"
	fi

	if echo "$CONTAINERS" | grep -q "oauth-proxy"; then
	  echo "✗ FAIL: Legacy oauth-proxy sidecar still present (RHOAI 2.x)"
	else
	  echo "✓ PASS: Legacy oauth-proxy sidecar removed"
	fi

	echo "Containers found: $CONTAINERS"
