#!/bin/bash
# ADR-015: Remove cluster-related tests from app_test.go

# Show which lines to remove (for review)
echo "Lines referencing Cluster in app_test.go:"
grep -n "Cluster\|cluster\|ModeCluster\|ModeStandalone" cmd/opengslb/app_test.go

echo ""
echo "Tests to remove:"
echo "  - Lines 370-407: Cluster Mode Tests section header and standalone config with cluster section"
echo "  - Lines 408-430: clusterConfigContent constant"
echo "  - Lines 459-463: IsStandaloneMode/IsClusterMode checks in TestApplicationInitialize_StandaloneMode"
echo "  - Lines 475-538: TestApplicationInitialize_ClusterMode, TestApplication_IsLeader_ClusterMode"
echo "  - Lines 542-679: All TestValidateClusterFlags_* tests"

echo ""
echo "Recommended: Manually edit cmd/opengslb/app_test.go to remove these tests"
echo "Or delete the entire file and recreate with only valid tests"
