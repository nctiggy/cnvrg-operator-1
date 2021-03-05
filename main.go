package main

import (
	"fmt"
	mlopsv1 "github.com/cnvrg-operator/api/v1"
	"github.com/cnvrg-operator/controllers"
	"github.com/go-logr/zapr"
	"github.com/markbates/pkger"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"os"
	ctrl "sigs.k8s.io/controller-runtime"
	"strings"
)

type param struct {
	name      string
	shorthand string
	value     interface{}
	usage     string
	required  bool
}

var (
	scheme            = runtime.NewScheme()
	setupLog          = ctrl.Log.WithName("setup")
	rootParams        = []param{}
	runOperatorParams = []param{
		{name: "metrics-addr", shorthand: "", value: ":8080", usage: "The address the metric endpoint binds to."},
		{name: "enable-leader-election", shorthand: "", value: false, usage: "Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager."},
		{name: "dry-run", shorthand: "", value: false, usage: "Only parse templates, without applying"},
		{name: "verbose", shorthand: "v", value: false, usage: "Verbose output"},
		{name: "deploy-depended-crds", shorthand: "", value: true, usage: "Deploy depended (external) CRDs automatically"},
		{name: "own-istio-resources", shorthand: "", value: true, usage: "Watch for istio resources"},
		{name: "own-openshift-resources", shorthand: "", value: false, usage: "Watch for OpenShift resources"},
		{name: "own-prometheus-resources", shorthand: "", value: true, usage: "Watch for Prometheus resources"},
	}
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = mlopsv1.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

// Root cmd and params
var rootCmd = &cobra.Command{
	Use:   "cnvrg-operator",
	Short: "cnvrg-operator - K8s operator for deploying cnvrg stack",
}

var runOperatorCmd = &cobra.Command{
	Use:   "run",
	Short: "Run cnvrg operator",
	Run: func(cmd *cobra.Command, args []string) {
		loggerMgr := initZapLog()
		loggerMgr.Sugar()
		zap.ReplaceGlobals(loggerMgr)
		runOperator()
	},
}

func initZapLog() *zap.Logger {
	config := zap.NewDevelopmentConfig()
	config.Level = zap.NewAtomicLevelAt(zapcore.InfoLevel)
	if viper.GetBool("verbose") {
		config.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
	}
	config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	config.EncoderConfig.TimeKey = "timestamp"
	logger, _ := config.Build()
	return logger
}

func setParams(params []param, command *cobra.Command) {
	for _, param := range params {
		switch v := param.value.(type) {
		case int:
			command.PersistentFlags().IntP(param.name, param.shorthand, v, param.usage)
		case string:
			command.PersistentFlags().StringP(param.name, param.shorthand, v, param.usage)
		case bool:
			command.PersistentFlags().BoolP(param.name, param.shorthand, v, param.usage)
		}
		if err := viper.BindPFlag(param.name, command.PersistentFlags().Lookup(param.name)); err != nil {
			panic(err)
		}
	}
}

func runOperator() {
	ctrl.SetLogger(zapr.NewLogger(initZapLog()))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: viper.GetString("metrics-addr"),
		Port:               9443,
		LeaderElection:     viper.GetBool("enable-leader-election"),
		LeaderElectionID:   "99748453.cnvrg.io",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = (&controllers.CnvrgAppReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("CnvrgApp"),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "CnvrgApp")
		os.Exit(1)
	}

	// +kubebuilder:scaffold:builder
	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func setupCommands() {
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	// Setup commands
	setParams(runOperatorParams, runOperatorCmd)
	setParams(rootParams, rootCmd)
	rootCmd.AddCommand(runOperatorCmd)
}

func informPkger() {
	pkger.Include("/pkg/pg/tmpl")
	pkger.Include("/pkg/networking/tmpl")
	pkger.Include("/pkg/controlplan/tmpl")
}

func main() {
	informPkger()
	setupCommands()
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}