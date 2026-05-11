// ingress.go
package cmd

import (
	"context"
	"fmt"
	"strings"
	"os"

	"github.com/Mahmut-Nihat/kubectl-helper/cmd/pkg"
	"github.com/pkg/errors"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// ingressCmd defines the Cobra command for interactively selecting an Ingress
// and viewing the related services and pods.
var ingressCmd = &cobra.Command{
	Use:   "ingress",
	Short: "Select an Ingress and view related service and pod information",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Initialize logger with info level
		pkg.Init("info")

		// Load Kubernetes client configuration
		kubeconfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			clientcmd.NewDefaultClientConfigLoadingRules(),
			&clientcmd.ConfigOverrides{},
		)
		restCfg, err := kubeconfig.ClientConfig()
		if err != nil {
			return fmt.Errorf("failed to get Kubernetes config: %w", err)
		}

		// Create a new clientset using the config
		clientset, err := kubernetes.NewForConfig(restCfg)
		if err != nil {
			return fmt.Errorf("failed to create Kubernetes client: %w", err)
		}

		// Fetch all Ingress resources across all namespaces
		ingresses, err := clientset.NetworkingV1().Ingresses("").List(context.Background(), metav1.ListOptions{})
		if err != nil {
			return fmt.Errorf("failed to list ingresses: %w", err)
		}
		if len(ingresses.Items) == 0 {
			fmt.Println("No Ingresses found.")
			return nil
		}

		// Prepare a simplified ingress list for selection
		type SimpleIngress struct {
			Name      string
			Namespace string
		}
		var ingressList []SimpleIngress
		for _, ing := range ingresses.Items {
			ingressList = append(ingressList, SimpleIngress{
				Name:      ing.Name,
				Namespace: ing.Namespace,
			})
		}

		// Prompt the user to select an Ingress
		prompt := promptui.Select{
			Label:     "Select Ingress",
			Items:     ingressList,
			Templates: pkg.IngressTemplate,
			Size:      5,
		}
		index, _, err := prompt.Run()
		if err != nil {
			if err == promptui.ErrInterrupt {
				fmt.Println("\nProcess interrupted.")
				os.Exit(0)		
			}
			return errors.Wrap(err, "ingress selection failed")
			// return fmt.Errorf("ingress selection cancelled: %w", err)
		}
		selectedIngress := ingressList[index]

		// Retrieve the full ingress object to extract backend services
		fullIngress, err := clientset.NetworkingV1().Ingresses(selectedIngress.Namespace).Get(context.Background(), selectedIngress.Name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get ingress details: %w", err)
		}

		// Extract backend service names from ingress rules
		var serviceNames []string
		for _, rule := range fullIngress.Spec.Rules {
			if rule.HTTP != nil {
				for _, path := range rule.HTTP.Paths {
					if path.Backend.Service != nil {
						serviceNames = append(serviceNames, fmt.Sprintf("%s:%d", path.Backend.Service.Name, path.Backend.Service.Port.Number))
					}
				}
			}
		}
		if len(serviceNames) == 0 {
			fmt.Println("No services found in selected Ingress.")
			return nil
		}

		// Prompt the user to select a Service
		svcPrompt := promptui.Select{
			Label:     "Select Service (from Ingress)",
			Items:     serviceNames,
			Templates: pkg.ServiceTemplate,
			Size:      5,
		}
		_, selectedService, err := svcPrompt.Run()
		if err != nil {
			if err == promptui.ErrInterrupt {
				fmt.Println("\nProcess interrupted.")
				os.Exit(0)
			}
			return errors.Wrap(err, "service selection failed")
		}

		// Parse the selected service name
		svcParts := strings.Split(selectedService, ":")
		if len(svcParts) != 2 {
			return fmt.Errorf("unexpected service format")
		}
		svcName := svcParts[0]

		// Retrieve the selected Service object
		service, err := clientset.CoreV1().Services(selectedIngress.Namespace).Get(context.Background(), svcName, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get service object: %w", err)
		}

		// List Pods that match the Service's label selector
		labelSelector := metav1.FormatLabelSelector(&metav1.LabelSelector{
			MatchLabels: service.Spec.Selector,
		})
		pods, err := clientset.CoreV1().Pods(selectedIngress.Namespace).List(context.Background(), metav1.ListOptions{
			LabelSelector: labelSelector,
		})
		if err != nil {
			return fmt.Errorf("failed to list pods for service: %w", err)
		}
		if len(pods.Items) == 0 {
			fmt.Println("No pods found for the selected service.")
			return nil
		}

		// Prepare pod list for selection
		type PodChoice struct {
			Name           string
			IP             string
			NodeName       string
			Phase          string
			ContainerCount int
		}
		var podChoices []PodChoice
		for _, p := range pods.Items {
			podChoices = append(podChoices, PodChoice{
				Name:           p.Name,
				IP:             p.Status.PodIP,
				NodeName:       p.Spec.NodeName,
				Phase:          string(p.Status.Phase),
				ContainerCount: len(p.Spec.Containers),
			})
		}

		// Prompt the user to select a Pod
		podPrompt := promptui.Select{
			Label:     "Select Pod",
			Items:     podChoices,
			Templates: pkg.PodTemplateIngress,
			Size:      5,
		}
		selectedIndex, _, err := podPrompt.Run()
		if err != nil {
			if err == promptui.ErrInterrupt {
				fmt.Println("\nProcess interrupted.")
				os.Exit(0)
			}
			// Handle other errors
			return errors.Wrap(err, "pod selection failed")
		}
		selectedPod := podChoices[selectedIndex]

		// Display selected pod information in a formatted table
		fmt.Printf("\n%-30s %-20s %-20s %-30s %-15s\n", "POD NAME", "NAMESPACE", "POD IP", "NODE NAME", "STATUS")
		fmt.Println(strings.Repeat("-", 120))
		fmt.Printf("%-30s %-20s %-20s %-30s %-15s\n",
			selectedPod.Name, selectedIngress.Namespace, selectedPod.IP, selectedPod.NodeName, selectedPod.Phase)

		return nil
	},
}

// Register the ingress command with the root command
func init() {
	RootCmd.AddCommand(ingressCmd)
}
