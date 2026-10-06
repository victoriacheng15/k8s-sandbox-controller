package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/blushft/go-diagrams/diagram"
	"github.com/blushft/go-diagrams/nodes/k8s"
)

func main() {
	d, err := diagram.New(
		diagram.Filename("architecture"),
		diagram.Label("\n\n\nk8s-sandbox-controller Architecture"),
		diagram.Direction("LR"),
		diagram.WithAttribute("pad", "1.0"),
		diagram.WithAttribute("nodesep", "0.75"),
		diagram.WithAttribute("ranksep", "0.75"),
		diagram.WithAttribute("fontsize", "24"),
	)
	if err != nil {
		log.Fatalf("failed to create diagram: %v", err)
	}

	nodeFontSize := func(fs string) diagram.NodeOption {
		return func(o *diagram.NodeOptions) {
			if o.Attributes == nil {
				o.Attributes = make(map[string]string)
			}
			o.Attributes["fontsize"] = fs
		}
	}

	nodeStyle := []diagram.NodeOption{
		diagram.Width(2.0),
		diagram.Height(2.8),
		nodeFontSize("16"),
	}

	// 1. Operator & Control Plane
	crd := k8s.Others.Crd(append(nodeStyle, diagram.NodeLabel("\n\n\n\nSandbox CRD\n(platform.sandbox.dev/v1alpha1)"))...)
	controller := k8s.Controlplane.CM(append(nodeStyle, diagram.NodeLabel("\n\n\n\nSandbox Controller\n(Reconcile Loop)"))...)
	cel := k8s.Controlplane.Api(append(nodeStyle, diagram.NodeLabel("\n\n\n\nCEL Admission Policy\n(ValidatingAdmissionPolicy)"))...)

	clusterOptions := func(m string, fontsize string) diagram.GroupOption {
		return func(o *diagram.GroupOptions) {
			if o.Attributes == nil {
				o.Attributes = make(map[string]string)
			}
			o.Attributes["margin"] = m
			o.Attributes["fontsize"] = fontsize
		}
	}

	// 2. Child Resources Group
	nsGroup := diagram.NewGroup("Dedicated Child Namespace (sbx-<name>)", clusterOptions("35", "16"))
	ns := k8s.Group.Ns(append(nodeStyle, diagram.NodeLabel("\n\n\n\nNamespace\nsandbox.dev/managed: 'true'"))...)
	quota := k8s.Clusterconfig.Quota(append(nodeStyle, diagram.NodeLabel("\n\n\n\nResourceQuota\nsbx-quota"))...)
	limits := k8s.Clusterconfig.Limits(append(nodeStyle, diagram.NodeLabel("\n\n\n\nLimitRange\nsbx-limits"))...)
	netpol := k8s.Network.Netpol(append(nodeStyle, diagram.NodeLabel("\n\n\n\nNetworkPolicy\nsbx-isolation"))...)
	sandboxPods := k8s.Compute.Pod(append(nodeStyle, diagram.NodeLabel("\n\n\n\nSandbox Pods"))...)

	nsGroup.Add(ns, quota, limits, netpol, sandboxPods)
	d.Group(nsGroup)

	// 3. Downstream Lifecycle & Observability
	phases := k8s.Others.Crd(append(nodeStyle, diagram.NodeLabel("\n\n\n\nStatus & Phases"))...)
	ttlFinalizer := k8s.Controlplane.Kubelet(append(nodeStyle, diagram.NodeLabel("\n\n\n\nTTL & Finalizer Cleanup"))...)
	observability := k8s.Controlplane.KProxy(append(nodeStyle, diagram.NodeLabel("\n\n\n\nObservability\n(Metrics & Logs)"))...)

	// 4. Connect Relationships
	d.Connect(crd, controller)
	d.Connect(controller, ns)
	d.Connect(controller, quota)
	d.Connect(controller, limits)
	d.Connect(controller, netpol)
	d.Connect(ns, sandboxPods)
	d.Connect(cel, sandboxPods)
	d.Connect(sandboxPods, phases)
	d.Connect(phases, ttlFinalizer)
	d.Connect(ttlFinalizer, observability)

	if err := d.Render(); err != nil {
		log.Fatalf("failed to render diagram: %v", err)
	}

	// Find project root
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("failed to get working directory: %v", err)
	}
	projectRoot := cwd
	if filepath.Base(cwd) == "hack" {
		projectRoot = filepath.Dir(cwd)
	}

	targetDir := filepath.Join(projectRoot, "docs", "assets")
	targetPNG := filepath.Join(targetDir, "architecture.png")
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		log.Fatalf("failed to create target directory: %v", err)
	}

	// Run dot inside the working directory where icons are present
	workingDir := "go-diagrams"
	if _, err := os.Stat(workingDir); os.IsNotExist(err) {
		workingDir = filepath.Join("hack", "go-diagrams")
	}

	dotCmd := exec.Command("dot", "-Tpng", "architecture.dot", "-o", targetPNG)
	dotCmd.Dir = workingDir
	if out, err := dotCmd.CombinedOutput(); err != nil {
		log.Fatalf("failed to execute dot command: %v\noutput: %s", err, string(out))
	}

	// Remove temporary go-diagrams directory
	_ = os.RemoveAll(workingDir)

	fmt.Printf("Generated diagram at %s\n", targetPNG)
}
