package importcmd

import (
	"fmt"
	"github.com/jenkins-x/jx/pkg/cmd/helper"
	"github.com/jenkins-x/jx/pkg/cmd/templates"
	"github.com/jenkins-x/jx/pkg/config"
	"github.com/jenkins-x/jx/pkg/log"
	"github.com/jenkins-x/jx/pkg/util"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"io/ioutil"
	"os"
	"path/filepath"
	"sigs.k8s.io/yaml"
)

// ImportOptions options struct for jx import
type ImportTravisOptions struct {
	RepoURL string
	Dir string
	*ImportOptions
}

// Travis - representation of a Travis YAML
type Travis struct {
	Language   string    `json:"language,omitempty"`
}


var (
	convertLong = templates.LongDesc(`
		Converts a Travis project to a jenkins x project and then imports into a cluster
		`)
	convertExample = templates.Examples(`
		# convert the current folder
		jx import travis

		# convert a different folder
		jx import travis /foo/bar

		# convert a Git repository from a URL
		jx import travis --url https://github.com/jenkins-x/spring-boot-web-example.git

		`)
)

// NewCmdImportTravis the cobra command for jx import
func NewCmdImportTravis(commonOpts *ImportOptions) *cobra.Command {
	options := &ImportTravisOptions{
		ImportOptions: commonOpts,
	}
	cmd := &cobra.Command{
		Use:     "travis",
		Short:   "Convert and import a travis project into Jenkins X",
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
func (options *ImportTravisOptions) Run() error {
	log.Logger().Info("Converting travis yaml")

	err := determineWorkingDir(options)
	if err != nil {
		return errors.Wrapf(err, "failed to run jx import travis")
	}

	travis, err := loadTravisSchema(options)
	log.Logger().Errorf("Error is %s", err)
	if err != nil {
		return errors.Wrapf(err, "failed to run jx import travis")
	}

	log.Logger().Infof("Travis schema loaded %s", travis)

	if travis != nil {
		err = buildJenkinsXSchema(options, travis)
		if err != nil {
			return errors.Wrapf(err, "failed to run jx import travis")
		}

		// now run the jx import
		err = options.ImportOptions.Run()
		if err != nil {
			return errors.Wrapf(err, "failed to run jx import travis")
		}
	}
	return err
}

// loadTravisSchema loads a specific project YAML configuration file
func loadTravisSchema(options *ImportTravisOptions) (*Travis, error) {
	fileName := filepath.Join(options.Dir, ".travis.yml")
	log.Logger().Infof("Filename %s", fileName)
	exists, err := util.FileExists(fileName)

	if err != nil {
		return nil, errors.Wrapf(err, "failed to check if file exists %s", fileName)
	}
	if !exists {
		log.Logger().Infof("File does not exist %s", fileName)
		return nil, nil
	}

	log.Logger().Infof("File exists %b", exists)
	data, err := ioutil.ReadFile(fileName)
	if err != nil {
		return nil, fmt.Errorf("failed to load file %s due to %s", fileName, err)
	}

	log.Logger().Infof("Date is %s", data)
	travis := &Travis{}
	err = yaml.Unmarshal(data, &travis)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal YAML file %s due to %s", fileName, err)
	}
	log.Logger().Infof("Travis is %s", travis)
	return travis, nil
}


func determineWorkingDir(options *ImportTravisOptions) error {
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


func buildJenkinsXSchema(options *ImportTravisOptions, travis *Travis) error {

	projectConfig := &config.ProjectConfig{
		BuildPack: travis.Language,
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

