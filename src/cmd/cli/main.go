package main

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"antelope/pkg/sdk"

	"github.com/spf13/cobra"
)

var (
	version   = "dev"
	serverURL string
	client    *sdk.Client
)

func main() {
	rootCmd := &cobra.Command{
		Use:     "antelope-cli",
		Short:   "Antelope CLI - bioinformatics pipeline management",
		Version: version,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			client = sdk.New(serverURL)
		},
	}

	rootCmd.PersistentFlags().StringVar(&serverURL, "server", getEnvOrDefault("ANTELOPE_SERVER", "http://localhost:8086/api/v1"), "Antelope server URL")

	rootCmd.AddCommand(loginCmd())
	rootCmd.AddCommand(pipelinesCmd())
	rootCmd.AddCommand(jobsCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func loginCmd() *cobra.Command {
	var email, password string
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate with the Antelope server",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := client.Login(email, password); err != nil {
				return fmt.Errorf("login failed: %w", err)
			}
			fmt.Println("Login successful.")
			return nil
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "Email address")
	cmd.Flags().StringVar(&password, "password", "", "Password")
	_ = cmd.MarkFlagRequired("email")
	_ = cmd.MarkFlagRequired("password")
	return cmd
}

func pipelinesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pipelines",
		Short: "Manage pipelines",
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List available pipelines",
		RunE: func(cmd *cobra.Command, args []string) error {
			pipelines, err := client.ListPipelines()
			if err != nil {
				return err
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tVERSION\tSTATUS\tAUTHOR\tDESCRIPTION")
			for _, p := range pipelines {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", p.Name, p.Version, p.Status, p.Author, p.Description)
			}
			return w.Flush()
		},
	}

	cmd.AddCommand(listCmd)
	return cmd
}

func jobsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "jobs",
		Short: "Manage jobs",
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List your jobs",
		RunE: func(cmd *cobra.Command, args []string) error {
			jobs, err := client.ListJobs(1, 20)
			if err != nil {
				return err
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tPIPELINE\tVERSION\tSTATUS\tCREATED")
			for _, j := range jobs {
				fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\n", j.ID, j.PipelineName, j.PipelineVersion, j.Status, j.CreatedAt.Format("2006-01-02 15:04"))
			}
			return w.Flush()
		},
	}

	submitCmd := &cobra.Command{
		Use:   "submit",
		Short: "Submit a new job",
		RunE: func(cmd *cobra.Command, args []string) error {
			name, _ := cmd.Flags().GetString("pipeline")
			ver, _ := cmd.Flags().GetString("version")
			paramsStr, _ := cmd.Flags().GetString("params")

			var params any
			if paramsStr != "" {
				if err := json.Unmarshal([]byte(paramsStr), &params); err != nil {
					return fmt.Errorf("invalid params JSON: %w", err)
				}
			} else {
				params = map[string]any{}
			}

			err := client.SubmitJob(sdk.SubmitJobRequest{
				PipelineName:    name,
				PipelineVersion: ver,
				PipelineParams:  params,
			})
			if err != nil {
				return err
			}
			fmt.Println("Job submitted successfully.")
			return nil
		},
	}
	submitCmd.Flags().String("pipeline", "", "Pipeline name")
	submitCmd.Flags().String("version", "", "Pipeline version")
	submitCmd.Flags().String("params", "{}", "Pipeline parameters as JSON")
	_ = submitCmd.MarkFlagRequired("pipeline")
	_ = submitCmd.MarkFlagRequired("version")

	cmd.AddCommand(listCmd, submitCmd)
	return cmd
}

func getEnvOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
