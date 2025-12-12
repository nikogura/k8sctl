package k8sctl

import (
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/nikogura/k8s-cluster-manager/pkg/manager"
	"github.com/nikogura/k8s-cluster-manager/pkg/manager/aws"
	"github.com/nikogura/k8s-cluster-manager/pkg/manager/cloudflare"
	"github.com/nikogura/k8s-cluster-manager/pkg/manager/kubernetes"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"net/http"
	"os"
	"strings"
	"time"
)

var cfAPIToken string
var cfZoneID string

// SetCloudflareCredentials sets the Cloudflare API credentials for the package.
func SetCloudflareCredentials(apiToken, zoneID string) {
	cfAPIToken = apiToken
	cfZoneID = zoneID
}

type DescribeClusterBody struct {
	Verbose bool `json:"verbose"`
}

type NodeCreateBody struct {
	Name          string `json:"name"`
	Role          string `json:"role"`
	Verbose       bool   `json:"verbose"`
	CloudProvider string `json:"cloud_provider"`
	Type          string `json:"type"`
	Purpose       string `json:"purpose"`
}

type NodeDeleteBody struct {
	Name          string `json:"name"`
	Verbose       bool   `json:"verbose"`
	CloudProvider string `json:"cloud_provider"`
}

func (c *K8sCtlCommands) DescribeClusterHandler(ctx *gin.Context) {
	clusterName := ctx.Param("cluster")
	logrus.Infof("Listing cluster %s\n", clusterName)

	var body DescribeClusterBody

	err := json.NewDecoder(ctx.Request.Body).Decode(&body)
	if err != nil {
		err = errors.Wrapf(err, "unable to  decode request body.")
		_ = ctx.AbortWithError(http.StatusBadRequest, err)
		return
	}

	verbose := body.Verbose

	dnsManager := cloudflare.NewCloudFlareManager(cfZoneID, cfAPIToken)
	cm, err := aws.NewAWSClusterManager(ctx, clusterName, "", "", dnsManager, verbose)
	if err != nil {
		logrus.Errorf("Failed creating cluster manager: %s", err)
		_ = ctx.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	// Set up cost estimator with custom pricing if configured
	region := cm.Config.Region
	customPricing := getCustomPricing()
	costEstimator := aws.NewAWSPricingEstimator(region, customPricing)
	cm.SetCostEstimator(costEstimator)

	info, err := cm.DescribeCluster(clusterName)
	if err != nil {
		logrus.Errorf("Failed describing cluster: %s", err)
		_ = ctx.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	ctx.JSON(http.StatusOK, info)

}

func (c *K8sCtlCommands) CreateNodeHandler(ctx *gin.Context) {
	var body NodeCreateBody

	err := json.NewDecoder(ctx.Request.Body).Decode(&body)
	if err != nil {
		err = errors.Wrapf(err, "unable to  decode request body.")
		_ = ctx.AbortWithError(http.StatusBadRequest, err)
		return
	}

	// Extract variables
	clusterName := ctx.Param("cluster")
	nodeName := body.Name
	verbose := body.Verbose
	nodeRole := body.Role
	cloudProvider := strings.ToLower(body.CloudProvider)

	logrus.Infof("creating node %s with role %s in cluster %s provider %s", nodeName, body.Role, clusterName, cloudProvider)

	// Error out if we're doing anything other than AWS
	if cloudProvider != "aws" {
		providerErr := errors.New(fmt.Sprintf("Unsupported cloud provider: %s", cloudProvider))
		logrus.Errorf("Unsupported cloud provider %s: %s", cloudProvider, providerErr)
		_ = ctx.AbortWithError(http.StatusInternalServerError, providerErr)
		return
	}

	// Create the cloudflare manager
	dnsManager := cloudflare.NewCloudFlareManager(cfZoneID, cfAPIToken)
	cm, err := aws.NewAWSClusterManager(ctx, clusterName, "", "", dnsManager, verbose)
	if err != nil {
		logrus.Errorf("Failed creating cluster manager: %s", err)
		_ = ctx.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	// NB: the following could be more efficiently done at pod start up, but that would require loading ALL the configs for all clusters and roles.  Not bothering with that now.  This isn't a high speed app.  Loading at run time is acceptable for now.
	// develop the expected paths for this cluster and node role
	machineConfigPath := fmt.Sprintf("/etc/clusters/%s/%s/config.yaml", clusterName, nodeRole)
	nodeConfigPath := fmt.Sprintf("/etc/clusters/%s/%s/node-%s.yaml", clusterName, nodeRole, cloudProvider)
	patchConfigPath := fmt.Sprintf("/etc/clusters/%s/%s/patch.yaml", clusterName, nodeRole)

	logrus.Infof("Machine Config Path: %s", machineConfigPath)
	logrus.Infof("Node Config Path: %s", nodeConfigPath)
	logrus.Infof("Patch Path: %s", patchConfigPath)

	// Load the machine config
	configBytes, err := os.ReadFile(machineConfigPath)
	if err != nil {
		err = errors.Wrapf(err, "Failed loading machine config file %s", machineConfigPath)
		logrus.Errorf("failed reading machine config file %s: %s", machineConfigPath, err)
		_ = ctx.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	// Load the node config
	nodeConfigBytes, err := os.ReadFile(nodeConfigPath)
	if err != nil {
		err = errors.Wrapf(err, "Failed loading node config file %s", nodeConfigPath)
		logrus.Errorf("failed reading node config file %s: %s", nodeConfigPath, err)
		_ = ctx.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	// Create the AWS Node Config struct
	nodeConfig, err := aws.LoadAWSNodeConfig(nodeConfigBytes)
	if err != nil {
		err = errors.Wrapf(err, "failed making node struct from config %s", nodeConfigPath)
		logrus.Errorf("failed making node struct from config %s: %s", nodeConfigPath, err)
		_ = ctx.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	// Override the default instance type if a type was provided in the request.
	if body.Type != "" {
		nodeConfig.InstanceType = body.Type
		logrus.Infof("setting instance type to %q", body.Type)
	}

	// Load the machine config patch
	patchBytes, err := os.ReadFile(patchConfigPath)
	if err != nil {
		err = errors.Wrapf(err, "Failed loading patch file %s", patchConfigPath)
		logrus.Errorf("failed reading patch file %s: %s", patchConfigPath, err)
		_ = ctx.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	logrus.Infof("node config: %s", nodeConfig)

	// Actually create the node and attach it to the load balancers
	err = cm.CreateNode(nodeName, nodeRole, nodeConfig, configBytes, []string{string(patchBytes)}, body.Purpose)
	if err != nil {
		logrus.Errorf("error creating node: %s", err)
		_ = ctx.AbortWithError(http.StatusInternalServerError, err)
		return
	}
}

func (c *K8sCtlCommands) DeleteNodeHandler(ctx *gin.Context) {
	var body NodeDeleteBody

	err := json.NewDecoder(ctx.Request.Body).Decode(&body)
	if err != nil {
		err = errors.Wrapf(err, "unable to  decode request body.")
		_ = ctx.AbortWithError(http.StatusBadRequest, err)
		return
	}

	// Extract variables
	clusterName := ctx.Param("cluster")
	verbose := body.Verbose
	nodeName := body.Name
	cloudProvider := strings.ToLower(body.CloudProvider)

	logrus.Infof("deleting node %s in cluster %s provider %s", nodeName, clusterName, cloudProvider)

	// Error out if we're doing anything other than AWS
	if cloudProvider != "aws" {
		providerErr := errors.New(fmt.Sprintf("Unsupported cloud provider: %s", cloudProvider))
		logrus.Errorf("Unsupported cloud provider %s: %s", cloudProvider, providerErr)
		_ = ctx.AbortWithError(http.StatusInternalServerError, providerErr)
		return
	}

	// Create the cloudflare manager
	dnsManager := cloudflare.NewCloudFlareManager(cfZoneID, cfAPIToken)
	cm, err := aws.NewAWSClusterManager(ctx, clusterName, "", "", dnsManager, verbose)
	if err != nil {
		logrus.Errorf("Failed creating cluster manager: %s", err)
		_ = ctx.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	// Delete Node
	err = cm.DeleteNode(nodeName)
	if err != nil {
		logrus.Errorf("error deleting node %s: %s", nodeName, err)
		_ = ctx.AbortWithError(http.StatusInternalServerError, err)
		return
	}

}

func (c *K8sCtlCommands) GlassNodeHandler(ctx *gin.Context) {
	nodeName := ctx.Param("node")
	clusterName := ctx.Param("cluster")

	logrus.Infof("glassing node %s in cluster %s\n", nodeName, clusterName)

}

func (c *K8sCtlCommands) DescribeNodeHandler(ctx *gin.Context) {
	nodeName := ctx.Param("node")
	clusterName := ctx.Param("cluster")

	logrus.Infof("describing node %s in cluster %s\n", nodeName, clusterName)

}

func (c *K8sCtlCommands) ReconcileClusterHandler(ctx *gin.Context) {
	clusterName := ctx.Param("cluster")

	logrus.Infof("reconciling cluster %s\n", clusterName)

	var body struct {
		Verbose bool `json:"verbose"`
		FixTags bool `json:"fix_tags"`
	}

	err := json.NewDecoder(ctx.Request.Body).Decode(&body)
	if err != nil {
		err = errors.Wrapf(err, "unable to decode request body")
		_ = ctx.AbortWithError(http.StatusBadRequest, err)
		return
	}

	verbose := body.Verbose
	fixTags := body.FixTags

	// Create the cloudflare manager
	dnsManager := cloudflare.NewCloudFlareManager(cfZoneID, cfAPIToken)
	cm, err := aws.NewAWSClusterManager(ctx, clusterName, "", "", dnsManager, verbose)
	if err != nil {
		logrus.Errorf("Failed creating cluster manager: %s", err)
		_ = ctx.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	// Get cluster info
	clusterInfo, err := cm.DescribeCluster(clusterName)
	if err != nil {
		logrus.Errorf("Failed getting cluster info: %s", err)
		_ = ctx.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	// Get K8s nodes
	k8sNodes, err := kubernetes.ListNodes(ctx, verbose)
	if err != nil {
		logrus.Errorf("Failed listing Kubernetes nodes: %s", err)
		_ = ctx.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	// Get nodes potentially missing Cluster tag
	untaggedNodes, err := cm.GetNodesInSecurityGroup()
	if err != nil {
		logrus.Errorf("Failed checking for untagged nodes: %s", err)
		_ = ctx.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	// Build maps for comparison
	// Normalize EC2 names by stripping domain suffix for comparison
	ec2Map := make(map[string]bool)
	ec2FullNameMap := make(map[string]string) // short name -> full name
	for _, node := range clusterInfo.Nodes {
		shortName := stripDomainSuffix(node.Name)
		ec2Map[shortName] = true
		ec2FullNameMap[shortName] = node.Name
	}

	k8sMap := make(map[string]bool)
	for _, node := range k8sNodes {
		k8sMap[node] = true
	}

	lbTargetMap := make(map[string]bool)
	for _, lb := range clusterInfo.LoadBalancers {
		for _, target := range lb.Targets {
			shortName := stripDomainSuffix(target.Name)
			lbTargetMap[shortName] = true
		}
	}

	// Collect all issues
	type ReconcileResult struct {
		UntaggedNodes    []string `json:"untagged_nodes,omitempty"`
		EC2NotInK8s      []string `json:"ec2_not_in_k8s,omitempty"`
		K8sNotInEC2      []string `json:"k8s_not_in_ec2,omitempty"`
		EC2NotInLB       []string `json:"ec2_not_in_lb,omitempty"`
		FixedTags        bool     `json:"fixed_tags"`
		Message          string   `json:"message"`
		TotalIssuesFound int      `json:"total_issues_found"`
	}

	result := ReconcileResult{}

	// Check for missing Cluster tags
	if len(untaggedNodes) > 0 {
		for _, node := range untaggedNodes {
			result.UntaggedNodes = append(result.UntaggedNodes, fmt.Sprintf("%s (%s)", node.Name, node.ID))
		}

		if fixTags {
			instanceIDs := make([]string, len(untaggedNodes))
			for i, node := range untaggedNodes {
				instanceIDs[i] = node.ID
			}
			err = cm.FixMissingClusterTags(instanceIDs)
			if err != nil {
				logrus.Errorf("Failed fixing tags: %s", err)
				_ = ctx.AbortWithError(http.StatusInternalServerError, err)
				return
			}
			result.FixedTags = true
		}
	}

	// Check for EC2 not in K8s
	for _, node := range clusterInfo.Nodes {
		shortName := stripDomainSuffix(node.Name)
		if !k8sMap[shortName] {
			result.EC2NotInK8s = append(result.EC2NotInK8s, node.Name)
		}
	}

	// Check for K8s not in EC2
	for _, node := range k8sNodes {
		if !ec2Map[node] {
			result.K8sNotInEC2 = append(result.K8sNotInEC2, node)
		}
	}

	// Check for EC2 not in any LB
	for _, node := range clusterInfo.Nodes {
		shortName := stripDomainSuffix(node.Name)
		if !lbTargetMap[shortName] {
			result.EC2NotInLB = append(result.EC2NotInLB, node.Name)
		}
	}

	// Calculate total issues
	result.TotalIssuesFound = len(result.UntaggedNodes) + len(result.EC2NotInK8s) + len(result.K8sNotInEC2) + len(result.EC2NotInLB)

	if result.TotalIssuesFound == 0 {
		result.Message = "No discrepancies found - cluster state is consistent"
	} else {
		result.Message = fmt.Sprintf("Found %d issue(s) in cluster state", result.TotalIssuesFound)
	}

	ctx.JSON(http.StatusOK, result)
}

func (c *K8sCtlCommands) MonitorClusterHandler(ctx *gin.Context) {
	clusterName := ctx.Param("cluster")

	logrus.Infof("monitoring cluster %s\n", clusterName)

	var body struct {
		Verbose  bool `json:"verbose"`
		Interval int  `json:"interval"`
	}

	err := json.NewDecoder(ctx.Request.Body).Decode(&body)
	if err != nil {
		err = errors.Wrapf(err, "unable to decode request body")
		_ = ctx.AbortWithError(http.StatusBadRequest, err)
		return
	}

	verbose := body.Verbose
	interval := body.Interval
	if interval <= 0 {
		interval = 60 // Default to 60 seconds
	}

	// Create the cloudflare manager
	dnsManager := cloudflare.NewCloudFlareManager(cfZoneID, cfAPIToken)
	cm, err := aws.NewAWSClusterManager(ctx, clusterName, "", "", dnsManager, verbose)
	if err != nil {
		logrus.Errorf("Failed creating cluster manager: %s", err)
		_ = ctx.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	// Set up response writer for streaming
	ctx.Writer.Header().Set("Content-Type", "text/plain")
	ctx.Writer.Header().Set("Transfer-Encoding", "chunked")
	ctx.Writer.WriteHeader(http.StatusOK)

	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	// Run initial check immediately
	monitorOnce(ctx, cm, clusterName, verbose)

	// Then run on interval
	for {
		select {
		case <-ticker.C:
			monitorOnce(ctx, cm, clusterName, verbose)
		case <-ctx.Request.Context().Done():
			return
		}
	}
}

func monitorOnce(ctx *gin.Context, cm *aws.AWSClusterManager, clusterName string, verbose bool) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	writeOutput(ctx, fmt.Sprintf("[%s] Checking cluster health...\n", timestamp))

	// Get cluster info
	clusterInfo, err := cm.DescribeCluster(clusterName)
	if err != nil {
		writeOutput(ctx, fmt.Sprintf("❌ ERROR: Failed getting cluster info: %s\n\n", err))
		return
	}

	// Get K8s nodes
	k8sNodes, err := kubernetes.ListNodes(ctx, verbose)
	if err != nil {
		writeOutput(ctx, fmt.Sprintf("❌ ERROR: Failed listing Kubernetes nodes: %s\n\n", err))
		return
	}

	// Get nodes potentially missing Cluster tag
	untaggedNodes, err := cm.GetNodesInSecurityGroup()
	if err != nil {
		writeOutput(ctx, fmt.Sprintf("❌ ERROR: Failed checking for untagged nodes: %s\n\n", err))
		return
	}

	// Build maps for comparison
	// Normalize EC2 names by stripping domain suffix for comparison
	ec2Map := make(map[string]bool)
	for _, node := range clusterInfo.Nodes {
		shortName := stripDomainSuffix(node.Name)
		ec2Map[shortName] = true
	}

	k8sMap := make(map[string]bool)
	for _, node := range k8sNodes {
		k8sMap[node] = true
	}

	lbTargetMap := make(map[string]bool)
	unhealthyTargets := make([]string, 0)
	for _, lb := range clusterInfo.LoadBalancers {
		for _, target := range lb.Targets {
			shortName := stripDomainSuffix(target.Name)
			lbTargetMap[shortName] = true
			if target.State != "healthy" {
				unhealthyTargets = append(unhealthyTargets, fmt.Sprintf("%s/%s:%d (%s)", lb.Name, target.Name, target.Port, target.State))
			}
		}
	}

	// Count issues
	issueCount := 0

	// Check for unhealthy targets
	if len(unhealthyTargets) > 0 {
		issueCount++
		writeOutput(ctx, fmt.Sprintf("  ⚠ Unhealthy Load Balancer Targets: %d\n", len(unhealthyTargets)))
		for _, target := range unhealthyTargets {
			writeOutput(ctx, fmt.Sprintf("    - %s\n", target))
		}
	}

	// Check for missing Cluster tags
	if len(untaggedNodes) > 0 {
		issueCount++
		writeOutput(ctx, fmt.Sprintf("  ⚠ Instances Missing Cluster Tag: %d\n", len(untaggedNodes)))
		for _, node := range untaggedNodes {
			writeOutput(ctx, fmt.Sprintf("    - %s (%s)\n", node.Name, node.ID))
		}
	}

	// Check for EC2 not in K8s
	notInK8s := make([]string, 0)
	for _, node := range clusterInfo.Nodes {
		shortName := stripDomainSuffix(node.Name)
		if !k8sMap[shortName] {
			notInK8s = append(notInK8s, node.Name)
		}
	}
	if len(notInK8s) > 0 {
		issueCount++
		writeOutput(ctx, fmt.Sprintf("  ⚠ EC2 Instances Not in Kubernetes: %d\n", len(notInK8s)))
		for _, name := range notInK8s {
			writeOutput(ctx, fmt.Sprintf("    - %s\n", name))
		}
	}

	// Check for K8s not in EC2
	notInEC2 := make([]string, 0)
	for _, node := range k8sNodes {
		if !ec2Map[node] {
			notInEC2 = append(notInEC2, node)
		}
	}
	if len(notInEC2) > 0 {
		issueCount++
		writeOutput(ctx, fmt.Sprintf("  ⚠ Kubernetes Nodes Not in EC2: %d\n", len(notInEC2)))
		for _, name := range notInEC2 {
			writeOutput(ctx, fmt.Sprintf("    - %s\n", name))
		}
	}

	// Check for EC2 not in any LB
	notInLB := make([]string, 0)
	for _, node := range clusterInfo.Nodes {
		shortName := stripDomainSuffix(node.Name)
		if !lbTargetMap[shortName] {
			notInLB = append(notInLB, node.Name)
		}
	}
	if len(notInLB) > 0 {
		issueCount++
		writeOutput(ctx, fmt.Sprintf("  ⚠ EC2 Instances Not in Any Load Balancer: %d\n", len(notInLB)))
		for _, name := range notInLB {
			writeOutput(ctx, fmt.Sprintf("    - %s\n", name))
		}
	}

	// Summary
	if issueCount == 0 {
		writeOutput(ctx, fmt.Sprintf("  ✓ All systems healthy - EC2: %d, K8s: %d, LB Targets: %d\n", len(clusterInfo.Nodes), len(k8sNodes), len(lbTargetMap)))
	} else {
		writeOutput(ctx, fmt.Sprintf("  Found %d issue(s)\n", issueCount))
	}

	writeOutput(ctx, "\n")
}

func writeOutput(ctx *gin.Context, message string) {
	_, _ = ctx.Writer.WriteString(message)
	ctx.Writer.Flush()
}

// stripDomainSuffix removes domain suffix from node names for comparison.
// E.g., "cluster1-cp-1.example.com" -> "cluster1-cp-1".
func stripDomainSuffix(name string) (shortName string) {
	parts := strings.Split(name, ".")
	shortName = parts[0]
	return shortName
}

// AuthCheckHandler handles authentication check requests.
func (c *K8sCtlCommands) AuthCheckHandler(ctx *gin.Context) {
	// If we reached here, authentication was successful (middleware passed)
	ctx.JSON(http.StatusOK, gin.H{
		"status":  "authenticated",
		"message": "Authentication successful",
	})
}

// UpgradeClusterHandler handles cluster upgrade requests.
func (c *K8sCtlCommands) UpgradeClusterHandler(ctx *gin.Context) {
	clusterName := ctx.Param("cluster")

	logrus.Infof("upgrading cluster %s\n", clusterName)

	var body struct {
		Version           string `json:"version"`
		ControlPlaneFirst bool   `json:"control_plane_first"`
		MaxConcurrent     int    `json:"max_concurrent"`
		Preserve          bool   `json:"preserve"`
		Stage             bool   `json:"stage"`
		WaitBetween       int    `json:"wait_between"`
		DryRun            bool   `json:"dry_run"`
		UpdateSecrets     bool   `json:"update_secrets"`
		Verbose           bool   `json:"verbose"`
	}

	err := json.NewDecoder(ctx.Request.Body).Decode(&body)
	if err != nil {
		err = errors.Wrapf(err, "unable to decode request body")
		_ = ctx.AbortWithError(http.StatusBadRequest, err)
		return
	}

	if body.Version == "" {
		err = errors.New("version is required")
		_ = ctx.AbortWithError(http.StatusBadRequest, err)
		return
	}

	verbose := body.Verbose

	// Create the cloudflare manager
	dnsManager := cloudflare.NewCloudFlareManager(cfZoneID, cfAPIToken)
	cm, err := aws.NewAWSClusterManager(ctx, clusterName, "", "", dnsManager, verbose)
	if err != nil {
		logrus.Errorf("Failed creating cluster manager: %s", err)
		_ = ctx.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	// Set upgrade options
	options := manager.UpgradeOptions{
		ControlPlaneFirst: body.ControlPlaneFirst,
		MaxConcurrent:     body.MaxConcurrent,
		Preserve:          body.Preserve,
		Stage:             body.Stage,
		WaitBetween:       time.Duration(body.WaitBetween) * time.Second,
		DryRun:            body.DryRun,
		UpdateSecrets:     body.UpdateSecrets,
	}

	// Perform upgrade
	result, err := cm.UpgradeCluster(body.Version, options)
	if err != nil {
		logrus.Errorf("Failed upgrading cluster: %s", err)
		_ = ctx.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	ctx.JSON(http.StatusOK, result)
}

// UpgradeNodeHandler handles single node upgrade requests.
func (c *K8sCtlCommands) UpgradeNodeHandler(ctx *gin.Context) {
	clusterName := ctx.Param("cluster")
	nodeName := ctx.Param("node")

	logrus.Infof("upgrading node %s in cluster %s\n", nodeName, clusterName)

	var body struct {
		Version       string `json:"version"`
		Preserve      bool   `json:"preserve"`
		Stage         bool   `json:"stage"`
		DryRun        bool   `json:"dry_run"`
		UpdateSecrets bool   `json:"update_secrets"`
		Verbose       bool   `json:"verbose"`
	}

	err := json.NewDecoder(ctx.Request.Body).Decode(&body)
	if err != nil {
		err = errors.Wrapf(err, "unable to decode request body")
		_ = ctx.AbortWithError(http.StatusBadRequest, err)
		return
	}

	if body.Version == "" {
		err = errors.New("version is required")
		_ = ctx.AbortWithError(http.StatusBadRequest, err)
		return
	}

	verbose := body.Verbose

	// Create the cloudflare manager
	dnsManager := cloudflare.NewCloudFlareManager(cfZoneID, cfAPIToken)
	cm, err := aws.NewAWSClusterManager(ctx, clusterName, "", "", dnsManager, verbose)
	if err != nil {
		logrus.Errorf("Failed creating cluster manager: %s", err)
		_ = ctx.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	// Set upgrade options
	options := manager.UpgradeOptions{
		Preserve:      body.Preserve,
		Stage:         body.Stage,
		DryRun:        body.DryRun,
		UpdateSecrets: body.UpdateSecrets,
	}

	// Perform upgrade
	result, err := cm.UpgradeNode(nodeName, body.Version, options)
	if err != nil {
		logrus.Errorf("Failed upgrading node: %s", err)
		_ = ctx.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	ctx.JSON(http.StatusOK, result)
}

// SecretsSyncHandler handles secrets sync requests.
func (c *K8sCtlCommands) SecretsSyncHandler(ctx *gin.Context) {
	clusterName := ctx.Param("cluster")

	logrus.Infof("syncing secrets for cluster %s\n", clusterName)

	var body struct {
		Role    string `json:"role"`
		DryRun  bool   `json:"dry_run"`
		Verbose bool   `json:"verbose"`
	}

	err := json.NewDecoder(ctx.Request.Body).Decode(&body)
	if err != nil {
		err = errors.Wrapf(err, "unable to decode request body")
		_ = ctx.AbortWithError(http.StatusBadRequest, err)
		return
	}

	verbose := body.Verbose

	// Create the cloudflare manager
	dnsManager := cloudflare.NewCloudFlareManager(cfZoneID, cfAPIToken)
	cm, err := aws.NewAWSClusterManager(ctx, clusterName, "", "", dnsManager, verbose)
	if err != nil {
		logrus.Errorf("Failed creating cluster manager: %s", err)
		_ = ctx.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	// Get cluster info to determine current versions
	clusterInfo, err := cm.DescribeCluster(clusterName)
	if err != nil {
		logrus.Errorf("Failed getting cluster info: %s", err)
		_ = ctx.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	// Determine which roles to sync
	rolesToSync := []string{"controlplane", "worker"}
	if body.Role != "" {
		rolesToSync = []string{body.Role}
	}

	type SyncResult struct {
		Role          string `json:"role"`
		CurrentAMI    string `json:"current_ami"`
		Version       string `json:"version"`
		UpdatedAMI    string `json:"updated_ami,omitempty"`
		UpdatedConfig bool   `json:"updated_config"`
		DryRun        bool   `json:"dry_run"`
	}

	results := make([]SyncResult, 0)

	for _, role := range rolesToSync {
		// Find a node with this role to get the current version
		var targetNode *manager.NodeInfo
		for i := range clusterInfo.Nodes {
			if strings.Contains(strings.ToLower(clusterInfo.Nodes[i].Name), "cp") && role == "controlplane" {
				targetNode = &clusterInfo.Nodes[i]
				break
			} else if !strings.Contains(strings.ToLower(clusterInfo.Nodes[i].Name), "cp") && role == "worker" {
				targetNode = &clusterInfo.Nodes[i]
				break
			}
		}

		if targetNode == nil {
			logrus.Warnf("No nodes found with role %s, skipping", role)
			continue
		}

		result := SyncResult{
			Role:   role,
			DryRun: body.DryRun,
		}

		// TODO: Get actual version from node via Talos API
		// For now, return placeholder response
		result.Version = "unknown"
		result.CurrentAMI = targetNode.ID

		if !body.DryRun {
			// TODO: Implement actual secret update
			result.UpdatedConfig = false
		}

		results = append(results, result)
	}

	ctx.JSON(http.StatusOK, gin.H{
		"cluster": clusterName,
		"results": results,
	})
}

// getCustomPricing returns custom pricing overrides for AWS instance types.
// This allows for custom negotiated rates, reserved instances, or savings plans.
// Pricing should be configured per deployment via environment variables or configuration files.
func getCustomPricing() (customPricing map[string]float64) {
	customPricing = make(map[string]float64)

	// Example: Load custom pricing from environment or config
	// customPricing["m5.large"] = 0.08
	// customPricing["c5.xlarge"] = 0.15

	// This keeps sensitive pricing data out of the codebase
	// TODO: Implement pricing configuration loader if needed

	return customPricing
}
