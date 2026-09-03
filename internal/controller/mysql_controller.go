/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"reflect"

	databasev1alpha1 "github.com/suellenmorrissey2461986jxe-maker/mysql-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const mysqlName = "mysql"

// MySQLReconciler reconciles a MySQL object.
type MySQLReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=database.ops.example.com,resources=mysqls,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=database.ops.example.com,resources=mysqls/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=database.ops.example.com,resources=mysqls/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete

func (r *MySQLReconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	mysql := &databasev1alpha1.MySQL{}
	if err := r.Get(ctx, req.NamespacedName, mysql); err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(1).Info("MySQL resource no longer exists")
			return ctrl.Result{}, nil
		}

		logger.Error(err, "Failed to get MySQL resource")
		return ctrl.Result{}, err
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mysql.Name,
			Namespace: mysql.Namespace,
		},
	}

	operation, err := controllerutil.CreateOrUpdate(
		ctx,
		r.Client,
		deployment,
		func() error {
			if !deployment.CreationTimestamp.IsZero() &&
				!metav1.IsControlledBy(deployment, mysql) {
				return fmt.Errorf(
					"deployment %s/%s already exists and is not controlled by MySQL",
					deployment.Namespace,
					deployment.Name,
				)
			}

			if err := controllerutil.SetControllerReference(
				mysql,
				deployment,
				r.Scheme,
			); err != nil {
				return err
			}

			selectorLabels := map[string]string{
				"app.kubernetes.io/name":     mysqlName,
				"app.kubernetes.io/instance": mysql.Name,
			}

			deployment.Labels = map[string]string{
				"app.kubernetes.io/name":       mysqlName,
				"app.kubernetes.io/instance":   mysql.Name,
				"app.kubernetes.io/managed-by": "mysql-operator",
			}

			deployment.Spec.Replicas = &mysql.Spec.Replicas
			deployment.Spec.Selector = &metav1.LabelSelector{
				MatchLabels: selectorLabels,
			}
			deployment.Spec.Template.Labels = selectorLabels
			deployment.Spec.Template.Spec.Containers = []corev1.Container{
				{
					Name:                     mysqlName,
					Image:                    mysql.Spec.Image,
					ImagePullPolicy:          corev1.PullIfNotPresent,
					TerminationMessagePath:   corev1.TerminationMessagePathDefault,
					TerminationMessagePolicy: corev1.TerminationMessageReadFile,
					Ports: []corev1.ContainerPort{
						{
							Name:          mysqlName,
							ContainerPort: 3306,
							Protocol:      corev1.ProtocolTCP,
						},
					},
					Env: []corev1.EnvVar{
						{
							Name:  "MYSQL_DATABASE",
							Value: mysql.Spec.DatabaseName,
						},
						{
							// 仅用于当前学习实验，后续将替换为 Secret。
							Name:  "MYSQL_ALLOW_EMPTY_PASSWORD",
							Value: "yes",
						},
					},
				},
			}

			return nil
		},
	)
	if err != nil {
		logger.Error(err, "Failed to reconcile MySQL Deployment")
		return ctrl.Result{}, err
	}

	if operation != controllerutil.OperationResultNone {
		logger.Info(
			"Reconciled MySQL Deployment",
			"operation", operation,
			"deployment", types.NamespacedName{
				Name:      deployment.Name,
				Namespace: deployment.Namespace,
			},
		)
	}

	readyReplicas := deployment.Status.ReadyReplicas
	phase := "Creating"
	conditionStatus := metav1.ConditionFalse
	reason := "DeploymentProgressing"
	message := fmt.Sprintf(
		"Deployment has %d/%d ready replicas",
		readyReplicas,
		mysql.Spec.Replicas,
	)

	if readyReplicas == mysql.Spec.Replicas {
		phase = "Running"
		conditionStatus = metav1.ConditionTrue
		reason = "DeploymentAvailable"
		message = "All MySQL replicas are ready"
	}

	return r.updateStatus(
		ctx,
		mysql,
		phase,
		readyReplicas,
		conditionStatus,
		reason,
		message,
	)
}

func (r *MySQLReconciler) updateStatus(
	ctx context.Context,
	mysql *databasev1alpha1.MySQL,
	phase string,
	readyReplicas int32,
	conditionStatus metav1.ConditionStatus,
	reason string,
	message string,
) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	original := mysql.DeepCopy()

	mysql.Status.Phase = phase
	mysql.Status.ReadyReplicas = readyReplicas
	mysql.Status.Message = message
	mysql.Status.ObservedGeneration = mysql.Generation

	meta.SetStatusCondition(
		&mysql.Status.Conditions,
		metav1.Condition{
			Type:               "Ready",
			Status:             conditionStatus,
			ObservedGeneration: mysql.Generation,
			Reason:             reason,
			Message:            message,
		},
	)

	if reflect.DeepEqual(original.Status, mysql.Status) {
		logger.V(1).Info("MySQL status is already current")
		return ctrl.Result{}, nil
	}

	if err := r.Status().Update(ctx, mysql); err != nil {
		logger.Error(err, "Failed to update MySQL status")
		return ctrl.Result{}, err
	}

	logger.Info(
		"Updated MySQL status",
		"phase", mysql.Status.Phase,
		"readyReplicas", mysql.Status.ReadyReplicas,
		"generation", mysql.Generation,
	)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MySQLReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&databasev1alpha1.MySQL{}).
		Owns(&appsv1.Deployment{}).
		Named(mysqlName).
		Complete(r)
}
