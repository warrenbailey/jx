package importcmd

import (
	"fmt"
	"github.com/Pallinder/go-randomdata"
	"github.com/codeship/codeship-go"
	"github.com/jenkins-x/jx/pkg/log"
	"github.com/jenkins-x/jx/pkg/cmd/helper"
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
	*ImportOptions
}

// BuildService structure of BuildService object for a Pro Project
type BuildService struct {
	Image   string    `json:"image,omitempty"`
	Build   string    `json:"build,omitempty"`
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
		jx import codeship

		# convert a different folder
		jx import codeship /foo/bar

		# convert a Git repository from a URL
		jx import codeship --url https://github.com/jenkins-x/spring-boot-web-example.git

		`)
	)

// NewCmdImportCodeship the cobra command for jx import
func NewCmdImportCodeship(commonOpts *ImportOptions) *cobra.Command {
	options := &ConvertOptions{
		ImportOptions: commonOpts,
	}
	cmd := &cobra.Command{
		Use:     "codeship",
		Short:   "Convert and import a codeship pro project into Jenkins X",
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
	log.Logger().Info("Converting codeship yaml")

	err := determineWorkingDir(options)

	buildSteps, err := loadCodeShipBuildSteps(options)
	if err != nil {
		return errors.Wrapf(err, "failed load codeship build steps")
	}

	buildServices, err := loadCodeShipBuildServices(options)
	if err != nil {
		return errors.Wrapf(err, "failed load codeship build services")
	}

	err = buildJenkinsXSchema(options, buildSteps, buildServices)

	// now run the jx import
	err = options.ImportOptions.Run()
	if err != nil {
		return errors.Wrapf(err, "failed to run jx import")
	}

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
		//BuildPack: "none",
		PipelineConfig: &jenkinsfile.PipelineConfig{
			Agent: &syntax.Agent {
				Image: getAgentImage(buildSteps[0], buildServices),
			},
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

		image := getImage(buildStep, buildServices)
		if image != "" {
			step.Image = image
		} else {

			debug := addDebugStep()
			steps = append(steps, debug)

			dockerBuild, dockerImage := addDockerBuildStep()
			steps = append(steps, dockerBuild)
			step.Image = dockerImage
		}

		steps = append(steps, step)
	}

	return steps
}

func getAgentImage(buildStep BuildStep, buildServices map[string]BuildService) string {
	image := getImage(buildStep, buildServices)
	if image != "" {
		return image
	} else {
		return "jenkinsxio/jx:2.0.128"
	}
}


func getImage(buildStep BuildStep, buildServices map[string]BuildService) string {
	serviceId := buildStep.Service
	service := buildServices[serviceId]
	image := service.Image
	return image
}

func addDebugStep() *syntax.Step {
	step := &syntax.Step{}
	step.Image = "busybox"
	step.Name = "debug"
	step.Command = "cat"
	step.Arguments = []string {
		"/workspace/source/Dockerfile",
	}
	return step
}

func addDockerBuildStep() (*syntax.Step, string) {
	step := &syntax.Step{}
	step.Name = strings.ToLower(randomdata.SillyName())
	step.Image = "gcr.io/kaniko-project/executor:9912ccbf8d22bbafbf971124600fbb0b13b9cbd6"
	step.Command = "/kaniko/executor"
	destination := "gcr.io/jx-development/warrenbailey/importer/"+step.Name+":${inputs.params.version}"
	step.Arguments = []string{
		"--dockerfile=/workspace/source/Dockerfile",
		"--destination="+destination,
		"--context=/workspace/source"}

	return step, destination
}