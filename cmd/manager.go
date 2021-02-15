package main

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	mlopsv1 "github.com/cnvrg-operator/api/v1"
	"github.com/cnvrg-operator/controllers"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"os"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	// +kubebuilder:scaffold:imports
)

var (
	scheme            = runtime.NewScheme()
	setupLog          = ctrl.Log.WithName("setup")
	runOperatorParams = []param{
		{name: "metrics-addr", shorthand: "", value: ":8080", usage: "The address the metric endpoint binds to."},
		{name: "enable-leader-election", shorthand: "", value: false, usage: "Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager."},
	}
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = mlopsv1.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

// run operator cmd and params
var runOperatorCmd = &cobra.Command{
	Use:   "run",
	Short: "Run cnvrg operator",
	Run: func(cmd *cobra.Command, args []string) {
		runOperator()
	},
}

func runOperator() {
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))
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