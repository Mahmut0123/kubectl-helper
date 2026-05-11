// ip.go
package cmd

import (
	"bytes"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/Mahmut-Nihat/kubectl-helper/cmd/pkg"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/resource"
)

// PodInfo holds the essential Pod data we want to display.
type PodInfo struct {
	Name      string
	Namespace string
	IP        string
	NodeName  string
	NodeIP    string
}

// namespaceFlag holds the namespace requested by the user via -n/--namespace
var namespaceFlag string

// configFlags is used to handle kubeconfig-based flags.
var configFlags = genericclioptions.NewConfigFlags(true)

// ipCmd is the main Cobra command for listing Pods by partial name match.
var ipCmd = &cobra.Command{
	Use:   "ip [SEARCH_PATTERN]",
	Short: "List pods containing [SEARCH_PATTERN] in their name, along with IP and node info.",
	// We bind our custom runFunc for command execution.
	RunE: runFunc(configFlags),

	// Add flags to the
}

// Add the namespace flag to ipCmd right here.
func init() {
	// This registers the -n/--namespace flag with our ipCmd.
	ipCmd.Flags().StringVarP(&namespaceFlag, "namespace", "n", "",
		"Namespace to filter pods. Searches all namespaces if omitted.")
}

// runFunc returns a function that searches for pods (in one or all namespaces)
// and filters them by the provided SEARCH_PATTERN.
func runFunc(configFlags *genericclioptions.ConfigFlags) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return fmt.Errorf("please provide a search pattern, for example:\n  ./api-deneme ip nginx\nor:\n  ./api-deneme ip -n dev nginx")
		}
		searchTerm := args[0]

		// Decide if we use the namespaceFlag or all namespaces
		var rb *resource.Builder
		if namespaceFlag != "" {
			rb = resource.NewBuilder(configFlags).
				Unstructured().
				ResourceTypeOrNameArgs(true, "pods").
				NamespaceParam(namespaceFlag).
				ContinueOnError().
				Flatten()
		} else {
			rb = resource.NewBuilder(configFlags).
				Unstructured().
				ResourceTypeOrNameArgs(true, "pods").
				AllNamespaces(true).
				ContinueOnError().
				Flatten()
		}

		var matchingPods []PodInfo

		err := rb.Do().Visit(func(info *resource.Info, visitErr error) error {
			if visitErr != nil {
				return visitErr
			}
			podInfo, convertErr := convertObjectToPodInfo(info.Object)
			if convertErr != nil {
				return nil
			}
			if strings.Contains(strings.ToLower(podInfo.Name), strings.ToLower(searchTerm)) {
				matchingPods = append(matchingPods, podInfo)
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("failed to retrieve pods: %w", err)
		}

		if len(matchingPods) == 0 {
			fmt.Printf("No pods found matching the pattern: %s\n", searchTerm)
			return nil
		}

		printColoredTable(matchingPods)
		return nil
	}
}

// convertObjectToPodInfo attempts to convert the provided runtime.Object to PodInfo.
func convertObjectToPodInfo(obj runtime.Object) (PodInfo, error) {
	unstructuredObj, ok := obj.(*unstructured.Unstructured)
	if !ok {
		objMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		if err != nil {
			return PodInfo{}, fmt.Errorf("failed to convert object to unstructured: %w", err)
		}
		unstructuredObj = &unstructured.Unstructured{Object: objMap}
	}

	spec, specOK := unstructuredObj.Object["spec"].(map[string]interface{})
	status, statusOK := unstructuredObj.Object["status"].(map[string]interface{})
	if !specOK || !statusOK {
		return PodInfo{}, fmt.Errorf("object does not contain 'spec' or 'status' in expected format")
	}

	podName := unstructuredObj.GetName()
	podNamespace := unstructuredObj.GetNamespace()
	podIP, _ := status["podIP"].(string)
	nodeNameRaw := spec["nodeName"]
	nodeName := nodeNameRaw.(string)
	hostIPRaw := status["hostIP"]
	hostIP := hostIPRaw.(string)

	return PodInfo{
		Name:      podName,
		Namespace: podNamespace,
		IP:        podIP,
		NodeName:  nodeName,
		NodeIP:    hostIP,
	}, nil
}

// printColoredTable prints the table with properly aligned columns and colored headers.
func printColoredTable(pods []PodInfo) {
	var buf bytes.Buffer
	w := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)

	fmt.Println()
	fmt.Fprintln(w, "NAME\tNAMESPACE\tPOD IP\tNODE NAME\tNODE IP     ")
	for _, p := range pods {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", p.Name, p.Namespace, p.IP, p.NodeName, p.NodeIP)
	}
	w.Flush()

	lines := strings.Split(buf.String(), "\n")

	headerColor := color.New(color.FgCyan, color.Bold)
	underlineColor := color.New(color.FgHiGreen)
	ipColor := color.New(color.FgMagenta)
	nodeColor := color.New(color.FgWhite, color.Bold)

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
			nodeColor.Sprint(cols[3]),
			ipColor.Sprint(cols[4]),
		)
	}
}
