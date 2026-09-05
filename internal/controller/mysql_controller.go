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
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"reflect"

	databasev1alpha1 "github.com/suellenmorrissey2461986jxe-maker/mysql-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
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
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete

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

	secretName := mysql.Name + "-credentials"
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: mysql.Namespace,
		},
	}

	secretOperation, err := controllerutil.CreateOrUpdate(
		ctx,
		r.Client,
		secret,
		func() error {
			if !secret.CreationTimestamp.IsZero() &&
				!metav1.IsControlledBy(secret, mysql) {
				return fmt.Errorf(
					"secret %s/%s already exists and is not controlled by MySQL",
					secret.Namespace,
					secret.Name,
				)
			}

			if err := controllerutil.SetControllerReference(
				mysql,
				secret,
				r.Scheme,
			); err != nil {
				return err
			}

			secret.Labels = map[string]string{
				"app.kubernetes.io/name":       mysqlName,
				"app.kubernetes.io/instance":   mysql.Name,
				"app.kubernetes.io/managed-by": "mysql-operator",
			}
			secret.Type = corev1.SecretTypeOpaque

			if secret.Data == nil {
				secret.Data = map[string][]byte{}
			}

			if len(secret.Data["root-password"]) == 0 {
				password, err := generatePassword(24)
				if err != nil {
					return err
				}
				secret.Data["root-password"] = []byte(password)
			}

			return nil
		},
	)
	if err != nil {
		logger.Error(err, "Failed to reconcile MySQL credentials Secret")
		return ctrl.Result{}, err
	}

	if secretOperation != controllerutil.OperationResultNone {
		logger.Info(
			"Reconciled MySQL credentials Secret",
			"operation", secretOperation,
			"secret", types.NamespacedName{
				Name:      secret.Name,
				Namespace: secret.Namespace,
			},
		)
	}

	storageSize := mysql.Spec.StorageSize.DeepCopy()
	if storageSize.IsZero() {
		storageSize = resource.MustParse("1Gi")
	}

	pvcName := mysql.Name + "-data"
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: mysql.Namespace,
		},
	}

	pvcOperation, err := controllerutil.CreateOrUpdate(
		ctx,
		r.Client,
		pvc,
		func() error {
			if !pvc.CreationTimestamp.IsZero() &&
				!metav1.IsControlledBy(pvc, mysql) {
				return fmt.Errorf(
					"persistentvolumeclaim %s/%s already exists and is not controlled by MySQL",
					pvc.Namespace,
					pvc.Name,
				)
			}

			if err := controllerutil.SetControllerReference(
				mysql,
				pvc,
				r.Scheme,
			); err != nil {
				return err
			}

			pvc.Labels = map[string]string{
				"app.kubernetes.io/name":       mysqlName,
				"app.kubernetes.io/instance":   mysql.Name,
				"app.kubernetes.io/managed-by": "mysql-operator",
			}
			pvc.Spec.AccessModes = []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			}
			pvc.Spec.Resources = corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: storageSize,
				},
			}

			return nil
		},
	)
	if err != nil {
		logger.Error(err, "Failed to reconcile MySQL data PVC")
		return ctrl.Result{}, err
	}

	if pvcOperation != controllerutil.OperationResultNone {
		logger.Info(
			"Reconciled MySQL data PVC",
			"operation", pvcOperation,
			"pvc", types.NamespacedName{
				Name:      pvc.Name,
				Namespace: pvc.Namespace,
			},
		)
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
			deployment.Spec.Strategy.Type = appsv1.RecreateDeploymentStrategyType
			deployment.Spec.Strategy.RollingUpdate = nil
			deployment.Spec.Selector = &metav1.LabelSelector{
				MatchLabels: selectorLabels,
			}
			deployment.Spec.Template.Labels = selectorLabels
			deployment.Spec.Template.Spec.Volumes = []corev1.Volume{
				{
					Name: "data",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: pvcName,
						},
					},
				},
			}
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
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "data",
							MountPath: "/var/lib/mysql",
						},
					},
					Env: []corev1.EnvVar{
						{
							Name:  "MYSQL_DATABASE",
							Value: mysql.Spec.DatabaseName,
						},
						{
							Name: "MYSQL_ROOT_PASSWORD",
							ValueFrom: &corev1.EnvVarSource{
								SecretKeyRef: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: secretName,
									},
									Key: "root-password",
								},
							},
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

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mysql.Name,
			Namespace: mysql.Namespace,
		},
	}

	serviceOperation, err := controllerutil.CreateOrUpdate(
		ctx,
		r.Client,
		service,
		func() error {
			if !service.CreationTimestamp.IsZero() &&
				!metav1.IsControlledBy(service, mysql) {
				return fmt.Errorf(
					"service %s/%s already exists and is not controlled by MySQL",
					service.Namespace,
					service.Name,
				)
			}

			if err := controllerutil.SetControllerReference(
				mysql,
				service,
				r.Scheme,
			); err != nil {
				return err
			}

			service.Labels = map[string]string{
				"app.kubernetes.io/name":       mysqlName,
				"app.kubernetes.io/instance":   mysql.Name,
				"app.kubernetes.io/managed-by": "mysql-operator",
			}
			service.Spec.Type = corev1.ServiceTypeClusterIP
			service.Spec.Selector = map[string]string{
				"app.kubernetes.io/name":     mysqlName,
				"app.kubernetes.io/instance": mysql.Name,
			}
			service.Spec.Ports = []corev1.ServicePort{
				{
					Name:       mysqlName,
					Protocol:   corev1.ProtocolTCP,
					Port:       3306,
					TargetPort: intstr.FromInt32(3306),
				},
			}

			return nil
		},
	)
	if err != nil {
		logger.Error(err, "Failed to reconcile MySQL Service")
		return ctrl.Result{}, err
	}

	if serviceOperation != controllerutil.OperationResultNone {
		logger.Info(
			"Reconciled MySQL Service",
			"operation", serviceOperation,
			"service", types.NamespacedName{
				Name:      service.Name,
				Namespace: service.Namespace,
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

func generatePassword(byteLength int) (string, error) {
	randomBytes := make([]byte, byteLength)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", fmt.Errorf("generate random password: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(randomBytes), nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MySQLReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&databasev1alpha1.MySQL{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Owns(&corev1.Service{}).
		Named(mysqlName).
		Complete(r)
}
