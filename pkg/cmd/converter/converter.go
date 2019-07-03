package converter

import (
	"fmt"
	"github.com/codeship/codeship-go"
	"github.com/jenkins-x/jx/pkg/cmd/helper"
	"github.com/jenkins-x/jx/pkg/cmd/opts"
	"github.com/jenkins-x/jx/pkg/cmd/templates"
	"github.com/jenkins-x/jx/pkg/config"
	"github.com/jenkins-x/jx/pkg/jenkinsfile"
	"github.com/jenkins-x/jx/pkg/tekton/syntax"
	"github.com/jenkins-x/jx/pkg/util"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"io/ioutil"
	"os"
	"path/filepath"
	"sigs.k8s.io/yaml"
	"strings"
)

// ImportOptions options struct for jx import
type ConvertOptions struct {
	RepoURL string
	Dir string
	*opts.CommonOptions
}

// BuildService structure of BuildService object for a Pro Project
type BuildService struct {
	Image   string    `json:"image,omitempty"`
}

type BuildStep struct {
	Service string `json:"service,omitempty"`
	*codeship.BuildStep
}

var (
	convertLong = templates.LongDesc(`
		Converts a codeship pro project to a jenkins x project and then imports into a cluster
		`)
	convertExample = templates.Examples(`
		# convert the current folder
		jx convert

		# convert a different folder
		jx convert /foo/bar

		# convert a Git repository from a URL
		jx import --url https://github.com/jenkins-x/spring-boot-web-example.git

		`)
	)

// NewCmdConvert the cobra command for jx import
func NewCmdConvert(commonOpts *opts.CommonOptions) *cobra.Command {
	options := &ConvertOptions{
		CommonOptions: commonOpts,
	}
	cmd := &cobra.Command{
		Use:     "convert",
		Short:   "Convert and import a codeship pro project into Jenkins",
		Long:    convertLong,
		Example: convertExample,
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := options.Run()
			helper.CheckErr(err)
		},
	}
	cmd.Flags().StringVarP(&options.RepoURL, "url", "u", "", "The git clone URL to clone into the current directory and then import")
	return cmd
}

// Run executes the command
func (options *ConvertOptions) Run() error {
	println("Converting codeship yaml")

	err := determineWorkingDir(options)

	buildSteps, err := loadCodeShipBuildSteps(options)
	if err != nil {
		return err
	}
	buildServices, err := loadCodeShipBuildServices(options)
	if err != nil {
		return err
	}

	err = buildJenkinsXSchema(options, buildSteps, buildServices)

	return err
}

func determineWorkingDir(options *ConvertOptions) error {
	if options.Dir == "" {
		args := options.Args
		if len(args) > 0 {
			options.Dir = args[0]
		} else {
			dir, err := os.Getwd()
			if err != nil {
				return err
			}
			options.Dir = dir
		}
	}
	return nil
}

// LoadSchema loads a specific project YAML configuration file
func loadCodeShipBuildSteps(options *ConvertOptions) ([]BuildStep, error) {

	fileName := filepath.Join(options.Dir, "codeship-steps.yml")
	exists, err := util.FileExists(fileName)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to check if file exists %s", fileName)
	}
	if !exists {
		return nil, errors.Wrapf(err, "file does not exist %s", fileName)
	}

	var buildStep []BuildStep
	data, err := ioutil.ReadFile(fileName)
	if err != nil {
		return nil, fmt.Errorf("failed to load file %s due to %s", fileName, err)
	}

	err = yaml.Unmarshal(data, &buildStep)
	if err != nil {
		return buildStep, fmt.Errorf("failed to unmarshal YAML file %s due to %s", fileName, err)
	}
	return buildStep, nil
}

// LoadSchema loads a specific project YAML configuration file
func loadCodeShipBuildServices(options *ConvertOptions) (map[string]BuildService, error) {

	fileName := filepath.Join(options.Dir, "codeship-services.yml")
	exists, err := util.FileExists(fileName)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to check if file exists %s", fileName)
	}
	if !exists {
		return nil, errors.Wrapf(err, "file does not exist %s", fileName)
	}


	data, err := ioutil.ReadFile(fileName)
	fmt.Println(data)
	if err != nil {
		return nil, fmt.Errorf("failed to load file %s due to %s", fileName, err)
	}

	var buildServices map[string]BuildService

	err = yaml.Unmarshal(data, &buildServices)
	if err != nil {
		return buildServices, fmt.Errorf("failed to unmarshal YAML file %s due to %s", fileName, err)
	}

	return buildServices, nil
}

func buildJenkinsXSchema(options *ConvertOptions, buildSteps []BuildStep, buildServices map[string]BuildService) error {

	steps := createSteps(buildSteps, buildServices)

	projectConfig := &config.ProjectConfig{
		BuildPack: "None",
		PipelineConfig: &jenkinsfile.PipelineConfig{
			Pipelines: jenkinsfile.Pipelines{
				PullRequest: &jenkinsfile.PipelineLifecycles{
					Build: &jenkinsfile.PipelineLifecycle{
						Steps: steps,
					},
				},
				Release: &jenkinsfile.PipelineLifecycles{
					Build: &jenkinsfile.PipelineLifecycle{
						Steps: steps,
					},
				},
			},

		},
	}

	data, err := yaml.Marshal(&projectConfig)
	if err != nil {
		return fmt.Errorf("failed to marshall data to YAML file", err)
	}

	fileName := filepath.Join(options.Dir, "jenkins-x.yml")
	err = ioutil.WriteFile(fileName, data, 0666)
	if err != nil {
		return fmt.Errorf("failed to write file %s due to %s", fileName, err)
	}
	return nil
}

func createSteps(buildSteps []BuildStep, buildServices map[string]BuildService) []*syntax.Step {
	var steps []*syntax.Step

	for _, buildStep := range buildSteps {
		step := &syntax.Step{}

		step.Name = buildStep.Name

		commandAndArgs := strings.Fields(buildStep.Command)
		step.Command = commandAndArgs[0]
		step.Arguments = commandAndArgs[1:]

		serviceId := buildStep.Service

		fmt.Println(buildServices)
		fmt.Println("Service id")
		fmt.Print(serviceId)

		service := buildServices[serviceId]
		step.Image = service.Image

		steps = append(steps, step)
	}

	return steps
}