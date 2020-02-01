package cmd

import (
	"os"

	"github.com/ikedam/cloudbuild/internal"
	"github.com/ikedam/cloudbuild/internal/signal"
	"github.com/ikedam/cloudbuild/log"
	homedir "github.com/mitchellh/go-homedir"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/xerrors"
)

var cfgFile string

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "cloudbuild",
	Short: "cloudbuild is a client application for Google Cloud Build",
	Long: `Launch a build for Google Cloud Build:

TODO`,
	Args: cobra.ExactValidArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		initLevel()
		signal.WithSignalStacktrace(
			viper.GetBool("alwaysDump"),
			func() {
				submit := &internal.CloudBuildSubmit{}

				if err := func() error {
					if err := viper.Unmarshal(&submit.Config); err != nil {
						return internal.NewConfigError("Failed to parse configurations", err)
					}
					if err := submit.Config.ResolveDefaults(); err != nil {
						return err
					}
					submit.Config.SourceDir = args[0]
					log.WithField("configuration", &submit.Config).Trace("Initialized configuration")

					return submit.Execute()
				}(); err != nil {
					var buildResultError *internal.BuildResultError
					if xerrors.As(err, &buildResultError) {
						log.Errorf("Build failed with %+v", buildResultError.Status)
					} else {
						log.WithError(err).Error("Failed to run a build")
					}
					log.Exit(internal.ExitCodeForError(err))
				}
			},
		)
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		log.WithError(err).Errorf("Failed to launch the command")
		log.Exit(internal.ExitCodeConfigurationError)
	}
}

func init() {
	cobra.OnInitialize(initLevel)
	cobra.OnInitialize(initConfig)

	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	// rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.cloudbuild.yaml)")
	rootCmd.PersistentFlags().String("log-level", "info", "Log level.")
	viper.BindPFlag("logLevel", rootCmd.PersistentFlags().Lookup("log-level"))
	rootCmd.PersistentFlags().Bool("always-dump", false, "Print stack dump also for SIGHUP, SIGINT, and SIGTERM")
	viper.BindPFlag("alwaysDump", rootCmd.PersistentFlags().Lookup("always-dump"))

	rootCmd.Flags().String("project", "", "ID of Google Cloud Project.")
	viper.BindPFlag("project", rootCmd.Flags().Lookup("project"))
	rootCmd.Flags().String("gcs-source-staging-dir", "", "GCS directory to store source archives.")
	viper.BindPFlag("gcsSourceStagingDir", rootCmd.Flags().Lookup("gcs-source-staging-dir"))
	rootCmd.Flags().String("ignore-file", ".gcloudignore", "File to use instead of .gcloudignore. Can be relative to the source directory.")
	viper.BindPFlag("ignoreFile", rootCmd.Flags().Lookup("ignore-file"))
	rootCmd.Flags().StringP("config", "c", "cloudbuild.yaml", "File to use instead of cloudbuild.yaml")
	viper.BindPFlag("config", rootCmd.Flags().Lookup("config"))
	rootCmd.Flags().StringSliceP("substitutions", "s", []string{}, "key=value expression to replace keywords in cloudbuild.yaml. Accepts multiple times.")
	viper.BindPFlag("substitutions", rootCmd.Flags().Lookup("substitutions"))

	viper.SetDefault("pollingIntervalMsec", 500)
	viper.SetDefault("uploadTimeoutMsec", 5*60*1000)
	viper.SetDefault("maxUploadTryCount", 5)
	viper.SetDefault("cloudBuildTimeoutMsec", 10*1000)
	viper.SetDefault("maxStartBuildTryCount", 5)
	viper.SetDefault("maxGetBuildTryCount", 100)
	viper.SetDefault("readLogTimeoutMsec", 30*1000)
	viper.SetDefault("maxReadLogTryCount", 100)
}

// initLevel initializes the log level.
func initLevel() {
	if err := log.SetLevelByName(viper.GetString("logLevel")); err != nil {
		log.WithError(err).Error("Invalid log level specified.")
		log.Exit(internal.ExitCodeConfigurationError)
	}
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := homedir.Dir()
		if err != nil {
			log.WithError(err).Error("Failed to stat home directory.")
			log.Exit(internal.ExitCodeConfigurationError)
		}

		// Search config in home directory with name ".cloudbuildconfig" (without extension).
		viper.AddConfigPath(home)
		viper.SetConfigName(".cloudbuildconfig")
	}

	viper.AutomaticEnv() // read in environment variables that match
	viper.SetEnvPrefix("CLOUDBUILD")

	// Read /etc/cloudbuild/config.yaml
	globalFile := "/etc/cloudbuild/config.yaml"
	if _, err := os.Stat(globalFile); err == nil {
		func() {
			log.WithField("file", globalFile).Debug("reading global config")
			fd, err := os.Open(globalFile)
			if err != nil {
				log.WithError(err).WithField("file", globalFile).Warning("Failed to open global config")
				return
			}
			defer fd.Close()
			if err := viper.ReadConfig(fd); err != nil {
				log.WithError(err).WithField("file", globalFile).Warning("Failed to read global config")
				return
			}
		}()
	}

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		log.WithField("configfile", viper.ConfigFileUsed).Trace("Using config file")
	}
}
