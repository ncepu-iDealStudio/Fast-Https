package cmd

import (
	"fast-https/utils"
	"fast-https/utils/errHelper"
	"fast-https/utils/message"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "project_layout",
	Short: "fast-https",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		// Start doing things.
		message.Println("Start Server.....")
		utils.GetWaitGroup().Add(1)

		// check something on here

	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	errHelper.ErrExit(rootCmd.Execute())
}

func init() {
	//rootCmd.PersistentFlags().StringP("Port", "P", "8000", "配置文件名(注意-C为大写)")
	cobra.OnInitialize(initConfig)
}

func initConfig() {
	//port, err := rootCmd.Flags().GetString("Port")
	//errHelper.ErrExit(err)
}
