package controllers

import (
	"context"
	mlopsv1 "github.com/cnvrg-operator/api/v1"
	"github.com/cnvrg-operator/pkg/desired"
	"github.com/cnvrg-operator/pkg/pg"
	"github.com/go-logr/logr"
	"github.com/imdario/mergo"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
)

type CnvrgAppReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=mlops.cnvrg.io,resources=cnvrgapps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=mlops.cnvrg.io,resources=cnvrgapps/status,verbs=get;update;patch

func (r *CnvrgAppReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {

	r.Log.Info("starting reconciliation")
	desiredSpec, err := r.desiredSpec(req)
	if err != nil {
		return ctrl.Result{}, err
	}

	if desiredSpec == nil {
		return ctrl.Result{}, nil // probably spec was deleted, no need to reconcile
	}

	if err := r.apply(pg.State(desiredSpec), desiredSpec); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *CnvrgAppReconciler) desiredSpec(req ctrl.Request) (*mlopsv1.CnvrgApp, error) {
	ctx := context.Background()
	var cnvrgApp mlopsv1.CnvrgApp
	if err := r.Get(ctx, req.NamespacedName, &cnvrgApp); err != nil {
		r.Log.Info("unable to fetch CnvrgApp, probably cr was deleted")
		return nil, nil
	}
	desiredSpec := mlopsv1.CnvrgApp{Spec: mlopsv1.DefaultSpec}
	if err := mergo.Merge(&desiredSpec, cnvrgApp, mergo.WithOverride); err != nil {
		r.Log.Error(err, "can't merge")
		return nil, err
	}
	return &desiredSpec, nil
}

func (r *CnvrgAppReconciler) apply(desiredManifests []*desired.State, desiredSpec *mlopsv1.CnvrgApp) error {
	ctx := context.Background()
	for _, s := range desiredManifests {
		if err := s.GenerateDeployable(desiredSpec); err != nil {
			r.Log.Error(err, "error generating deployable", "name", s.Name)
			return err
		}
		if err := ctrl.SetControllerReference(desiredSpec, s.Obj, r.Scheme); err != nil {
			r.Log.Error(err, "error setting controller reference", "name", s.Name)
			return err
		}
		err := r.Get(ctx, types.NamespacedName{Name: s.Name, Namespace: desiredSpec.Namespace}, s.Obj)
		if err != nil && errors.IsNotFound(err) {
			r.Log.Info("creating", "name", s.Name, "kind", s.GVR.Kind)
			if err := r.Create(ctx, s.Obj); err != nil {
				r.Log.Error(err, "error creating object", "name", s.Name)
				return err
			}
		}
	}
	return nil
}

func (r *CnvrgAppReconciler) SetupWithManager(mgr ctrl.Manager) error {

	deployments := &unstructured.Unstructured{}
	deployments.SetGroupVersionKind(schema.GroupVersionKind{Kind: "Deployment", Group: "", Version: "apps/v1"})

	services := &unstructured.Unstructured{}
	services.SetGroupVersionKind(schema.GroupVersionKind{Kind: "Service", Group: "", Version: "v1"})

	pvcs := &unstructured.Unstructured{}
	pvcs.SetGroupVersionKind(schema.GroupVersionKind{Kind: "PersistentVolumeClaim", Group: "", Version: "v1"})

	secrets := &unstructured.Unstructured{}
	secrets.SetGroupVersionKind(schema.GroupVersionKind{Kind: "Secret", Group: "", Version: "v1"})

	return ctrl.NewControllerManagedBy(mgr).
		For(&mlopsv1.CnvrgApp{}).
		Owns(&corev1.ConfigMap{}).
		Owns(deployments).
		Owns(services).
		Owns(pvcs).
		Owns(secrets).
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
