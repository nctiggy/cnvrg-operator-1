package controllers

import (
	"context"
	mlopsv1 "github.com/cnvrg-operator/api/v1"
	"github.com/cnvrg-operator/pkg/cnvrgapp/controlplan"
	"github.com/cnvrg-operator/pkg/cnvrgapp/ingress"
	"github.com/cnvrg-operator/pkg/cnvrgapp/logging"
	"github.com/cnvrg-operator/pkg/cnvrgapp/minio"
	"github.com/cnvrg-operator/pkg/cnvrgapp/pg"
	"github.com/cnvrg-operator/pkg/cnvrgapp/redis"
	"github.com/cnvrg-operator/pkg/desired"
	"github.com/go-logr/logr"
	"github.com/imdario/mergo"
	"github.com/spf13/viper"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"reflect"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"strings"
)

const CnvrgappFinalizer = "cnvrgapp.mlops.cnvrg.io/finalizer"

type CnvrgAppReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

var cnvrgAppLog logr.Logger

// +kubebuilder:rbac:groups=mlops.cnvrg.io,resources=cnvrgapps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=mlops.cnvrg.io,resources=cnvrgapps/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=*,resources=*,verbs=*

func (r *CnvrgAppReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {

	cnvrgAppLog = r.Log.WithValues("name", req.NamespacedName)
	cnvrgAppLog.Info("starting cnvrgapp reconciliation")

	equal, err := r.syncCnvrgAppSpec(req.NamespacedName)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !equal {
		return ctrl.Result{Requeue: true}, nil
	}

	cnvrgApp, err := r.getCnvrgAppSpec(req.NamespacedName)
	if err != nil {
		return ctrl.Result{}, err
	}
	if cnvrgApp == nil {
		return ctrl.Result{}, nil // probably spec was deleted, no need to reconcile
	}

	// Setup finalizer
	if cnvrgApp.ObjectMeta.DeletionTimestamp.IsZero() {
		if !containsString(cnvrgApp.ObjectMeta.Finalizers, CnvrgappFinalizer) {
			cnvrgApp.ObjectMeta.Finalizers = append(cnvrgApp.ObjectMeta.Finalizers, CnvrgappFinalizer)
			if err := r.Update(context.Background(), cnvrgApp); err != nil {
				cnvrgAppLog.Error(err, "failed to add finalizer")
				return ctrl.Result{}, err
			}
		}
	} else {
		if containsString(cnvrgApp.ObjectMeta.Finalizers, CnvrgappFinalizer) {
			r.updateStatusMessage(mlopsv1.STATUS_REMOVING, "removing cnvrg spec", cnvrgApp)
			if err := r.cleanup(cnvrgApp); err != nil {
				return ctrl.Result{}, err
			}
			cnvrgApp.ObjectMeta.Finalizers = removeString(cnvrgApp.ObjectMeta.Finalizers, CnvrgappFinalizer)
			if err := r.Update(context.Background(), cnvrgApp); err != nil {
				cnvrgAppLog.Info("error in removing finalizer, checking if cnvrgApp object still exists")
				// if update was failed, make sure that cnvrgApp still exists
				spec, e := r.getCnvrgAppSpec(req.NamespacedName)
				if spec == nil && e == nil {
					return ctrl.Result{}, nil // probably spec was deleted, stop reconcile
				}
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	r.updateStatusMessage(mlopsv1.STATUS_RECONCILING, "reconciling", cnvrgApp)

	if err := r.applyManifests(cnvrgApp); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.triggerInfraReconciler(cnvrgApp, "add"); err != nil {
		return ctrl.Result{}, err
	}

	r.updateStatusMessage(mlopsv1.STATUS_HEALTHY, "successfully reconciled", cnvrgApp)
	cnvrgAppLog.Info("successfully reconciled")
	return ctrl.Result{}, nil
}

func (r *CnvrgAppReconciler) applyManifests(cnvrgApp *mlopsv1.CnvrgApp) error {

	// Ingress
	if err := desired.Apply(ingress.State(cnvrgApp), cnvrgApp, r.Client, r.Scheme, cnvrgAppLog); err != nil {
		r.updateStatusMessage(mlopsv1.STATUS_ERROR, err.Error(), cnvrgApp)
		return err
	}

	// ControlPlan
	if err := desired.Apply(controlplan.State(cnvrgApp), cnvrgApp, r.Client, r.Scheme, cnvrgAppLog); err != nil {
		r.updateStatusMessage(mlopsv1.STATUS_ERROR, err.Error(), cnvrgApp)
		return err
	}

	// Logging
	if err := desired.Apply(logging.State(cnvrgApp), cnvrgApp, r.Client, r.Scheme, cnvrgAppLog); err != nil {
		r.updateStatusMessage(mlopsv1.STATUS_ERROR, err.Error(), cnvrgApp)
		return err
	}

	// Redis
	if err := desired.Apply(redis.State(cnvrgApp), cnvrgApp, r.Client, r.Scheme, cnvrgAppLog); err != nil {
		r.updateStatusMessage(mlopsv1.STATUS_ERROR, err.Error(), cnvrgApp)
		return err
	}

	// PostgreSQL
	if err := desired.Apply(pg.State(cnvrgApp), cnvrgApp, r.Client, r.Scheme, cnvrgAppLog); err != nil {
		r.updateStatusMessage(mlopsv1.STATUS_ERROR, err.Error(), cnvrgApp)
		return err
	}

	// Minio
	if err := desired.Apply(minio.State(cnvrgApp), cnvrgApp, r.Client, r.Scheme, cnvrgAppLog); err != nil {
		r.updateStatusMessage(mlopsv1.STATUS_ERROR, err.Error(), cnvrgApp)
		return err
	}

	return nil
}

func (r *CnvrgAppReconciler) triggerInfraReconciler(cnvrgApp *mlopsv1.CnvrgApp, op string) error {

	cnvrgAppInfra := &mlopsv1.CnvrgInfraList{}

	if err := r.List(context.Background(), cnvrgAppInfra); err != nil {
		cnvrgAppLog.Error(err, "can't list CnvrgInfra objects")
		return err
	}

	if len(cnvrgAppInfra.Items) == 0 {
		cnvrgAppLog.Info("no CnvrgInfra objects was deployed, skipping infra reconciler")
		return nil
	}

	name := types.NamespacedName{
		Name:      cnvrgAppInfra.Items[0].Spec.InfraReconcilerCm,
		Namespace: cnvrgAppInfra.Items[0].Spec.CnvrgInfraNs,
	}

	cm := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cnvrgAppInfra.Items[0].Spec.InfraReconcilerCm,
			Namespace: cnvrgAppInfra.Items[0].Spec.CnvrgInfraNs},
	}

	if err := r.Get(context.Background(), name, cm); err != nil && errors.IsNotFound(err) {
		cnvrgAppLog.Info("infra reconciler cm does not exists, skipping", name, name)
		return nil
	} else if err != nil {
		cnvrgAppLog.Error(err, "can't get cm", "name", name)
		return err
	}

	if op == "add" {
		if cm.Data == nil {
			cm.Data = map[string]string{cnvrgApp.Namespace: cnvrgApp.Name}
		} else {
			cm.Data[cnvrgApp.Namespace] = cnvrgApp.Name
		}
	}
	if op == "remove" {
		delete(cm.Data, cnvrgApp.Namespace)
	}
	if err := r.Update(context.Background(), cm); err != nil {
		cnvrgAppLog.Error(err, "can't update cm", "cm", name)
		return err
	}

	return nil
}

func (r *CnvrgAppReconciler) updateStatusMessage(status mlopsv1.OperatorStatus, message string, cnvrgApp *mlopsv1.CnvrgApp) {
	if cnvrgApp.Status.Status == mlopsv1.STATUS_REMOVING {
		cnvrgAppLog.Info("skipping status update, current cnvrg spec under removing status...")
		return
	}
	ctx := context.Background()
	cnvrgApp.Status.Status = status
	cnvrgApp.Status.Message = message
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		err := r.Status().Update(ctx, cnvrgApp)
		return err
	})
	if err != nil {
		cnvrgAppLog.Error(err, "can't update status")
	}

	//// This check is to make sure that the status is indeed updated
	//// short reconciliations loop might cause status to be applied but not yet saved into BD
	//// and leads to error: "the object has been modified; please apply your changes to the latest version and try again"
	//// to avoid this error, fetch the object and compare the status
	//statusCheckAttempts := 3
	//for {
	//	cnvrgApp, err := r.getCnvrgAppSpec(types.NamespacedName{Namespace: cnvrgApp.Namespace, Name: cnvrgApp.Name})
	//	if err != nil {
	//		cnvrgAppLog.Error(err, "can't validate status update")
	//	}
	//	cnvrgAppLog.V(1).Info("expected status", "status", status, "message", message)
	//	cnvrgAppLog.V(1).Info("current status", "status", cnvrgApp.Status.Status, "message", cnvrgApp.Status.Message)
	//	if cnvrgApp.Status.Status == status && cnvrgApp.Status.Message == message {
	//		break
	//	}
	//	if statusCheckAttempts == 0 {
	//		cnvrgAppLog.Info("can't verify status update, status checks attempts exceeded")
	//		break
	//	}
	//	statusCheckAttempts--
	//	cnvrgAppLog.V(1).Info("validating status update", "attempts", statusCheckAttempts)
	//	time.Sleep(1 * time.Second)
	//}
}

func (r *CnvrgAppReconciler) syncCnvrgAppSpec(name types.NamespacedName) (bool, error) {

	cnvrgAppLog.Info("synchronizing cnvrgApp spec")

	// Fetch current cnvrgApp spec
	cnvrgApp, err := r.getCnvrgAppSpec(name)
	if err != nil {
		return false, err
	}
	if cnvrgApp == nil {
		return false, nil // probably cnvrgapp was removed
	}
	cnvrgAppLog = r.Log.WithValues("name", name, "ns", cnvrgApp.Namespace)

	// Get default cnvrgApp spec
	desiredSpec := mlopsv1.DefaultCnvrgAppSpec()

	// Merge current cnvrgApp spec into default spec ( make it indeed desiredSpec )
	if err := mergo.Merge(&desiredSpec, cnvrgApp.Spec, mergo.WithOverride); err != nil {
		cnvrgAppLog.Error(err, "can't merge")
		return false, err
	}

	equal := reflect.DeepEqual(desiredSpec, cnvrgApp.Spec)
	if !equal {
		cnvrgAppLog.Info("states are not equals, syncing and requeuing")
		cnvrgApp.Spec = desiredSpec
		if err := r.Update(context.Background(), cnvrgApp); err != nil && errors.IsConflict(err) {
			cnvrgAppLog.Info("conflict updating cnvrgApp object, requeue for reconciliations...")
			return true, nil
		} else if err != nil {
			return false, err
		}
		return equal, nil
	}

	cnvrgAppLog.Info("states are equals, no need to sync")
	return equal, nil
}

func (r *CnvrgAppReconciler) getCnvrgAppSpec(namespacedName types.NamespacedName) (*mlopsv1.CnvrgApp, error) {
	ctx := context.Background()
	var cnvrgApp mlopsv1.CnvrgApp
	if err := r.Get(ctx, namespacedName, &cnvrgApp); err != nil {
		if errors.IsNotFound(err) {
			cnvrgAppLog.Info("unable to fetch CnvrgApp, probably cr was deleted")
			return nil, nil
		}
		cnvrgAppLog.Error(err, "unable to fetch CnvrgApp")
		return nil, err
	}
	return &cnvrgApp, nil
}

func (r *CnvrgAppReconciler) cleanup(cnvrgApp *mlopsv1.CnvrgApp) error {

	cnvrgAppLog.Info("running finalizer cleanup")

	// remove cnvrg-db-init
	if err := r.cleanupDbInitCm(cnvrgApp); err != nil {
		return err
	}

	// update infra reconciler cm
	if err := r.triggerInfraReconciler(cnvrgApp, "remove"); err != nil {
		return err
	}

	return nil
}

func (r *CnvrgAppReconciler) cleanupDbInitCm(desiredSpec *mlopsv1.CnvrgApp) error {
	cnvrgAppLog.Info("running cnvrg-db-init cleanup")
	ctx := context.Background()
	dbInitCm := &v1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cnvrg-db-init", Namespace: desiredSpec.Namespace}}
	err := r.Delete(ctx, dbInitCm)
	if err != nil && errors.IsNotFound(err) {
		cnvrgAppLog.Info("no need to delete cnvrg-db-init, cm not found")
	} else {
		cnvrgAppLog.Error(err, "error deleting cnvrg-db-init")
		return err
	}
	return nil
}

func (r *CnvrgAppReconciler) SetupWithManager(mgr ctrl.Manager) error {
	cnvrgAppLog = r.Log.WithValues("initializing", "crds")

	p := predicate.Funcs{

		UpdateFunc: func(e event.UpdateEvent) bool {

			if reflect.TypeOf(&mlopsv1.CnvrgApp{}) == reflect.TypeOf(e.ObjectOld) {
				oldObject := e.ObjectOld.(*mlopsv1.CnvrgApp)
				newObject := e.ObjectNew.(*mlopsv1.CnvrgApp)
				// deleting cnvrg cr
				if !newObject.ObjectMeta.DeletionTimestamp.IsZero() {
					return true
				}
				shouldReconcileOnSpecChange := reflect.DeepEqual(oldObject.Spec, newObject.Spec) // cnvrgapp spec wasn't changed, assuming status update, won't reconcile
				cnvrgAppLog.V(1).Info("update received", "shouldReconcileOnSpecChange", shouldReconcileOnSpecChange)

				return !shouldReconcileOnSpecChange
			}
			return true
		},
	}

	cnvrgAppController := ctrl.
		NewControllerManagedBy(mgr).
		For(&mlopsv1.CnvrgApp{}).
		WithEventFilter(p)

	for _, v := range desired.Kinds {

		if strings.Contains(v.Group, "istio.io") && viper.GetBool("own-istio-resources") == false {
			continue
		}
		if strings.Contains(v.Group, "openshift.io") && viper.GetBool("own-openshift-resources") == false {
			continue
		}
		if strings.Contains(v.Group, "coreos.com") && viper.GetBool("own-prometheus-resources") == false {
			continue
		}
		u := &unstructured.Unstructured{}
		u.SetGroupVersionKind(v)
		cnvrgAppController.Owns(u)
	}

	return cnvrgAppController.
		WithOptions(controller.Options{MaxConcurrentReconciles: 1}).
		Complete(r)
}

func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func removeString(slice []string, s string) (result []string) {
	for _, item := range slice {
		if item == s {
			continue
		}
		result = append(result, item)
	}
	return
}
