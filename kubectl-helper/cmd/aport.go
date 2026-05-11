// aport.go
package cmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/Mahmut-Nihat/kubectl-helper/cmd/pkg"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

type ServiceInfo struct {
	Name      string
	Namespace string
	ClusterIP string
	NodePort  int32
	Protocol  string
}

var checkPort int

var aportCmd = &cobra.Command{
	Use:   "aport",
	Short: "List all NodePort services or check availability of a specific NodePort",
	RunE:  runAport,
}

func init() {
	aportCmd.Flags().IntVarP(&checkPort, "check", "c", 0, "Check if the given NodePort is in use")
}

func runAport(cmd *cobra.Command, args []string) error {
	kubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	services, usedPorts, err := getNodePortServices(clientset)
	if err != nil {
		return err
	}

	if checkPort > 0 {
		return checkNodePort(int32(checkPort), usedPorts)
	}

	printColoredServiceTable(services)
	return nil
}

// getNodePortServices collects all NodePort services and tracks used ports
func getNodePortServices(clientset *kubernetes.Clientset) ([]ServiceInfo, map[int32]bool, error) {
	svcs, err := clientset.CoreV1().Services("").List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list services: %w", err)
	}

	var infoList []ServiceInfo
	used := make(map[int32]bool)

	for _, svc := range svcs.Items {
		if svc.Spec.Type != corev1.ServiceTypeNodePort {
			continue
		}
		for _, p := range svc.Spec.Ports {
			infoList = append(infoList, ServiceInfo{
				Name:      svc.Name,
				Namespace: svc.Namespace,
				ClusterIP: svc.Spec.ClusterIP,
				NodePort:  p.NodePort,
				Protocol:  string(p.Protocol),
			})
			used[p.NodePort] = true
		}
	}

	sort.Slice(infoList, func(i, j int) bool {
		return infoList[i].NodePort < infoList[j].NodePort
	})

	return infoList, used, nil
}

// checkNodePort verifies if a given port is in use and suggests an alternative if needed
func checkNodePort(port int32, used map[int32]bool) error {
	fmt.Println()
	if used[port] {
		fmt.Printf("🔴 Port %d is already in use.\n\n", port)
		for p := int32(30000); p <= 32767; p++ {
			if !used[p] {
				fmt.Printf("💡 Suggested available port: %d\n\n", p)
				break
			}
		}
	} else {
		fmt.Printf("🟢 Port %d is available.\n\n", port)
	}
	return nil
}

// printColoredServiceTable renders a colored, aligned table of services
func printColoredServiceTable(svcs []ServiceInfo) {
	var buf bytes.Buffer
	w := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)

	// Header
    fmt.Println()
	fmt.Fprintln(w, "NAME\tNAMESPACE\tCLUSTER-IP\tNODEPORT\tPROTOCOL")

	for _, svc := range svcs {
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\n",
			svc.Name, svc.Namespace, svc.ClusterIP, svc.NodePort, svc.Protocol)
	}
	w.Flush()

	lines := strings.Split(buf.String(), "\n")

	headerColor := color.New(color.FgCyan, color.Bold)
	underlineColor := color.New(color.FgHiGreen)
	ipColor := color.New(color.FgMagenta)
	portColor := color.New(color.FgWhite, color.Bold)

	if len(lines) > 0 {
		headerColor.Println(lines[0])
		underlineColor.Println(pkg.BuildDashedLine(lines[0]))
	}

	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == "" {
			continue
		}
		cols := strings.Split(line, "\t")
		if len(cols) != 5 {
			fmt.Println(line)
			continue
		}
		fmt.Printf("%s\t%s\t%s\t%s\t%s\n",
			cols[0],
			cols[1],
			ipColor.Sprint(cols[2]),
			portColor.Sprint(cols[3]),
			cols[4],
		)
	}
	fmt.Println()
}
