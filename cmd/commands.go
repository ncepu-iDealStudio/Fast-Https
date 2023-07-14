package cmd

// var rootCmd = &cobra.Command{
// 	Use:   "fast-https",
// 	Short: "fast-https",
// 	Long:  `A simple command line fast-https`,
// 	Run: func(cmd *cobra.Command, args []string) {
// 		// Start doing things.
// 		message.Println("Start Server.....")
// 		utils.GetWaitGroup().Add(1)

// 		// check something on here

// 	},
// }

// // add a  command line parameter start
// var startCmd = &cobra.Command{
// 	Use:   "start",
// 	Short: "Start the app",
// 	Run: func(cmd *cobra.Command, args []string) {
// 		fmt.Println("Starting app...")
// 		// 启动程序
// 		start()
// 		fmt.Println("App started")

// 	},
// }

// // add a  command line parameter reload
// var reloadCmd = &cobra.Command{
// 	Use:   "start",
// 	Short: "Start the app",
// 	Run: func(cmd *cobra.Command, args []string) {
// 		fmt.Println("Reloading app...")
// 		// 重新载程序
// 		reload()
// 		fmt.Println("App Reloaded!")

// 	},
// }

// // add a  command line parameter stop
// var stopCmd = &cobra.Command{
// 	Use:   "stop",
// 	Short: "Stop the app",
// 	Run: func(cmd *cobra.Command, args []string) {
// 		fmt.Println("Stopping app...")
// 		// 停止程序
// 		stop()
// 		fmt.Println("App stopped")
// 	},
// }

// // add a  command line parameter restart
// var restartCmd = &cobra.Command{
// 	Use:   "restart",
// 	Short: "Restart the app",
// 	Run: func(cmd *cobra.Command, args []string) {
// 		fmt.Println("Restarting app...")
// 		// 停止程序
// 		stop()
// 		// 等待一段时间
// 		time.Sleep(1 * time.Second)
// 		// 启动程序
// 		start()
// 		fmt.Println("App restarted")
// 	},
// }

// // Execute adds all child commands to the root command and sets flags appropriately.
// // This is called by main.main(). It only needs to happen once to the rootCmd.
// func Execute1() {
// 	rootCmd.AddCommand(startCmd, reloadCmd, stopCmd, restartCmd)
// 	if err := rootCmd.Execute(); err != nil {
// 		fmt.Println(err)
// 		os.Exit(1)
// 	}
// 	errHelper.ErrExit(rootCmd.Execute())
// }

// func init() {
// 	//rootCmd.PersistentFlags().StringP("Port", "P", "8000", "配置文件名(注意-C为大写)")
// 	cobra.OnInitialize(initConfig)
// }

// func initConfig() {
// 	//port, err := rootCmd.Flags().GetString("Port")
// 	//errHelper.ErrExit(err)
// }

// func start() {
// 	// 这里编写启动Web服务器的代码
// 	// 比如启动一个 HTTP 服务器
// 	// http.ListenAndServe(":8080", nil)
// 	go func() {
// 		httpServer := http.Server{Addr: ":8080"} // 创建HTTP服务器
// 		if err := httpServer.ListenAndServe(); err != nil {
// 			fmt.Println("HTTP server stopped")
// 		}
// 	}()

// 	// 建立信号监听
// 	signals := make(chan os.Signal, 1)
// 	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

// 	// 等待信号
// 	<-signals
// }

// func stop() {
// 	// 这里可以编写停止程序的代码
// 	// 比如关闭 HTTP 服务器
// 	// httpServer.Close()
// }

// func reload() {
// 	// 这里可以编写停止程序的代码
// 	// 比如关闭 HTTP 服务器
// 	// httpServer.Close()
// }
