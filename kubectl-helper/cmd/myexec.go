// myexec.go
package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/manifoldco/promptui"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"golang.org/x/term"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"

	"github.com/Mahmut-Nihat/kubectl-helper/cmd/pkg"
)

var restConfig *rest.Config
var (
	namespace     string
	podFilter     string
	labelSelector string
	podName       string
	logLevel      string
)

// myexecCmd defines the CLI command structure
var myexecCmd = &cobra.Command{
	Use:   "myexec",
	Short: "Interactively select a pod/container and execute a shell or command",
	Long:  "A CLI tool that allows users to interact with Kubernetes pods and containers with optional filtering and shell execution.",
	RunE:  runMyExec(),
}

// runMyExec handles the main execution logic for the command
func runMyExec() func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		pkg.Init(logLevel)

		client, err := kubernetes.NewForConfig(restConfig)
		if err != nil {
			return errors.Wrap(err, "failed to create Kubernetes client")
		}

		podName, namespace, err := selectPod(client)
		if err != nil {
			return err
		}

		containerName, err := selectContainer(client, namespace, podName)
		if err != nil {
			return err
		}

		command, err := determineCommand(cmd, args)
		if err != nil {
			return err
		}

		return executeCommand(client, namespace, podName, containerName, command)
	}
}

// selectPod lists available pods and prompts the user to select one
func selectPod(client *kubernetes.Clientset) (string, string, error) {
	pods, err := client.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return "", "", errors.Wrap(err, "failed to list pods from Kubernetes API")
	}

	podItems := []struct {
		Name      string
		Namespace string
	}{}
	for _, pod := range pods.Items {
		if podFilter == "" || strings.Contains(pod.Name, podFilter) {
			podItems = append(podItems, struct {
				Name      string
				Namespace string
			}{
				Name:      pod.Name,
				Namespace: pod.Namespace,
			})
		}
	}

	if len(podItems) == 0 {
		return "", "", fmt.Errorf("no pods found matching the given filters in namespace '%s'", namespace)
	}

	prompt := promptui.Select{
		Label:     "Select Pod",
		Items:     podItems,
		Templates: pkg.PodTemplate,
		Size:      5,
	}

	index, _, err := prompt.Run()
	if err != nil {
		if err == promptui.ErrInterrupt {
			fmt.Println("\nProcess interrupted.")
			os.Exit(0)
		}
		return "", "", errors.Wrap(err, "pod selection failed")
	}

	selected := podItems[index]
	fmt.Printf("Selected Pod: %s\n", selected.Name)
	pkg.Infof("Selected pod: %s", selected.Name)
	return selected.Name, selected.Namespace, nil
}

// selectContainer lists available containers within a selected pod and prompts the user to select one
func selectContainer(client *kubernetes.Clientset, namespace, podName string) (string, error) {
	pod, err := client.CoreV1().Pods(namespace).Get(context.Background(), podName, metav1.GetOptions{})
	if err != nil {
		return "", errors.Wrapf(err, "failed to retrieve pod details for '%s'", podName)
	}

	containers := pod.Spec.Containers
	if len(containers) == 1 {
		return containers[0].Name, nil
	}

	containerNames := []string{}
	for _, c := range containers {
		containerNames = append(containerNames, c.Name)
	}

	prompt := promptui.Select{
		Label:     "Select Container",
		Items:     containerNames,
		Templates: pkg.ContainerTemplate,
		Size:      5,
	}

	index, _, err := prompt.Run()
	if err != nil {
		if err == promptui.ErrInterrupt {
			fmt.Println("\nProcess interrupted.")
			os.Exit(0)
		}
		return "", errors.Wrap(err, "container selection failed")
	}

	selected := containerNames[index]
	fmt.Printf("Selected Container: %s\n", selected)
	pkg.Infof("Selected container: %s", selected)
	return selected, nil
}

// determineCommand decides which command to execute inside the container
func determineCommand(cmd *cobra.Command, args []string) ([]string, error) {
	if len(args) > 0 {
		if cmd.Flags().ArgsLenAtDash() == -1 {
			return nil, fmt.Errorf("you must use '--' before specifying the shell or command (example: -- bash)")
		}

		if len(args) == 1 {
			return []string{"/bin/" + args[0]}, nil
		}
		return args, nil
	}
	return []string{"/bin/sh"}, nil
}

// executeCommand connects to the selected pod/container and streams the shell session
func executeCommand(client *kubernetes.Clientset, namespace, podName, containerName string, command []string) error {
	req := client.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).
		SubResource("exec").
		Param("stdin", "true").
		Param("stdout", "true").
		Param("stderr", "true").
		Param("tty", "true").
		Param("container", containerName)

	for _, arg := range command {
		req = req.Param("command", arg)
	}

	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return errors.Wrap(err, "failed to set terminal into raw mode")
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	sizeQueue := &SizeQueue{}
	go sizeQueue.MonitorSize()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	executor, err := remotecommand.NewSPDYExecutor(restConfig, "POST", req.URL())
	if err != nil {
		return errors.Wrap(err, "failed to create SPDY executor for exec command")
	}

	err = executor.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdin:             os.Stdin,
		Stdout:            os.Stdout,
		Stderr:            os.Stderr,
		Tty:               true,
		TerminalSizeQueue: sizeQueue,
	})
	if err != nil {
		if _, ok := err.(interface{ ExitStatus() int }); ok {
			return nil
		}
		return errors.Wrap(err, "failed during exec session streaming")
	}

	return nil
}

// SizeQueue monitors terminal size changes for proper resizing
type SizeQueue struct {
	resize chan remotecommand.TerminalSize
}

// Next returns the next terminal size update
func (s *SizeQueue) Next() *remotecommand.TerminalSize {
	size, ok := <-s.resize
	if !ok {
		return nil
	}
	return &size
}

// MonitorSize watches for window resize signals and updates the terminal size
func (s *SizeQueue) MonitorSize() {
	s.resize = make(chan remotecommand.TerminalSize, 1)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGWINCH)
	defer signal.Stop(sig)

	for {
		width, height, err := term.GetSize(int(os.Stdin.Fd()))
		if err == nil {
			s.resize <- remotecommand.TerminalSize{
				Width:  uint16(width),
				Height: uint16(height),
			}
		}
		<-sig
	}
}

// init loads kubeconfig and binds CLI flags
func init() {
	kubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading kubeconfig: %v\n", err)
		os.Exit(1)
	}
	restConfig = config

	myexecCmd.Flags().StringVarP(&namespace, "namespace", "n", "", "Select namespace")
	myexecCmd.Flags().StringVarP(&podFilter, "filter", "f", "", "Filter pods by name substring")
	myexecCmd.Flags().StringVarP(&labelSelector, "selector", "l", "", "Filter pods by label selector (e.g. -l app=nginx)")
	myexecCmd.Flags().StringVarP(&podName, "pod-name", "p", "", "Select pod directly by name")
	myexecCmd.Flags().StringVar(&logLevel, "log-level", "error", "Set log level (trace, debug, info, warn, error, fatal, panic)")
}
