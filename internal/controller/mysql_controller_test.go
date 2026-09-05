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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	databasev1alpha1 "github.com/suellenmorrissey2461986jxe-maker/mysql-operator/api/v1alpha1"
)

var _ = Describe("MySQL Controller", func() {
	Context("When reconciling a resource", func() {
		const (
			resourceName      = "test-resource"
			resourceNamespace = "default"
		)

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: resourceNamespace,
		}
		mysql := &databasev1alpha1.MySQL{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind MySQL")
			err := k8sClient.Get(ctx, typeNamespacedName, mysql)
			if err != nil && errors.IsNotFound(err) {
				resource := &databasev1alpha1.MySQL{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: resourceNamespace,
					},
					Spec: databasev1alpha1.MySQLSpec{
						DatabaseName: "testdb",
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &databasev1alpha1.MySQL{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance MySQL")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &MySQLReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			By("Checking the status written by the reconciler")
			updated := &databasev1alpha1.MySQL{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, updated)).To(Succeed())

			Expect(updated.Status.Phase).To(Equal("Creating"))
			Expect(updated.Status.ReadyReplicas).To(Equal(int32(0)))
			Expect(updated.Status.ObservedGeneration).To(Equal(updated.Generation))
			Expect(updated.Status.Conditions).To(HaveLen(1))
			Expect(updated.Status.Conditions[0].Type).To(Equal("Ready"))
			Expect(updated.Status.Conditions[0].Status).To(Equal(metav1.ConditionFalse))

			By("Checking the Deployment created by the reconciler")
			deployment := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, deployment)).To(Succeed())

			By("Checking the data PVC created by the reconciler")
			pvcName := resourceName + "-data"
			pvcNamespacedName := types.NamespacedName{
				Name:      pvcName,
				Namespace: resourceNamespace,
			}
			pvc := &corev1.PersistentVolumeClaim{}
			Expect(k8sClient.Get(
				ctx,
				pvcNamespacedName,
				pvc,
			)).To(Succeed())

			Expect(pvc.Spec.AccessModes).To(Equal(
				[]corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteOnce,
				},
			))
			requestedStorage := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
			Expect(requestedStorage.String()).To(Equal("1Gi"))
			Expect(metav1.IsControlledBy(pvc, updated)).To(BeTrue())

			By("Checking the credentials Secret created by the reconciler")
			secretName := resourceName + "-credentials"
			secretNamespacedName := types.NamespacedName{
				Name:      secretName,
				Namespace: resourceNamespace,
			}
			secret := &corev1.Secret{}
			Expect(k8sClient.Get(ctx, secretNamespacedName, secret)).To(Succeed())

			originalPassword := append(
				[]byte(nil),
				secret.Data["root-password"]...,
			)
			Expect(originalPassword).NotTo(BeEmpty())
			Expect(metav1.IsControlledBy(secret, updated)).To(BeTrue())

			By("Checking that the Deployment references the credentials Secret")
			Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(1))

			var rootPasswordEnv *corev1.EnvVar
			for index := range deployment.Spec.Template.Spec.Containers[0].Env {
				env := &deployment.Spec.Template.Spec.Containers[0].Env[index]
				if env.Name == "MYSQL_ROOT_PASSWORD" {
					rootPasswordEnv = env
					break
				}
			}

			Expect(rootPasswordEnv).NotTo(BeNil())
			Expect(rootPasswordEnv.ValueFrom).NotTo(BeNil())
			Expect(rootPasswordEnv.ValueFrom.SecretKeyRef).NotTo(BeNil())
			Expect(rootPasswordEnv.ValueFrom.SecretKeyRef.Name).To(Equal(secretName))
			Expect(rootPasswordEnv.ValueFrom.SecretKeyRef.Key).To(Equal("root-password"))
			Expect(deployment.Spec.Replicas).NotTo(BeNil())
			Expect(*deployment.Spec.Replicas).To(Equal(int32(1)))
			Expect(deployment.Spec.Strategy.Type).To(Equal(
				appsv1.RecreateDeploymentStrategyType,
			))
			Expect(deployment.Spec.Strategy.RollingUpdate).To(BeNil())
			Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(1))
			Expect(deployment.Spec.Template.Spec.Containers[0].Image).To(Equal("mysql:8.4"))
			Expect(metav1.IsControlledBy(deployment, updated)).To(BeTrue())

			By("Checking that the Deployment mounts the data PVC")
			Expect(deployment.Spec.Template.Spec.Volumes).To(HaveLen(1))
			dataVolume := deployment.Spec.Template.Spec.Volumes[0]
			Expect(dataVolume.Name).To(Equal("data"))
			Expect(
				dataVolume.VolumeSource.PersistentVolumeClaim,
			).NotTo(BeNil())
			Expect(
				dataVolume.VolumeSource.PersistentVolumeClaim.ClaimName,
			).To(Equal(pvcName))

			container := deployment.Spec.Template.Spec.Containers[0]
			By("Checking the MySQL health probes")
			expectedProbeCommand := []string{
				"sh",
				"-c",
				`MYSQL_PWD="$MYSQL_ROOT_PASSWORD" mysqladmin ping -h 127.0.0.1 -uroot --silent`,
			}

			Expect(container.StartupProbe).NotTo(BeNil())
			Expect(container.StartupProbe.Exec).NotTo(BeNil())
			Expect(container.StartupProbe.Exec.Command).To(Equal(expectedProbeCommand))
			Expect(container.StartupProbe.PeriodSeconds).To(Equal(int32(5)))
			Expect(container.StartupProbe.TimeoutSeconds).To(Equal(int32(2)))
			Expect(container.StartupProbe.FailureThreshold).To(Equal(int32(30)))

			Expect(container.ReadinessProbe).NotTo(BeNil())
			Expect(container.ReadinessProbe.Exec).NotTo(BeNil())
			Expect(container.ReadinessProbe.Exec.Command).To(Equal(expectedProbeCommand))
			Expect(container.ReadinessProbe.PeriodSeconds).To(Equal(int32(5)))
			Expect(container.ReadinessProbe.TimeoutSeconds).To(Equal(int32(2)))
			Expect(container.ReadinessProbe.FailureThreshold).To(Equal(int32(3)))

			Expect(container.LivenessProbe).NotTo(BeNil())
			Expect(container.LivenessProbe.Exec).NotTo(BeNil())
			Expect(container.LivenessProbe.Exec.Command).To(Equal(expectedProbeCommand))
			Expect(container.LivenessProbe.PeriodSeconds).To(Equal(int32(10)))
			Expect(container.LivenessProbe.TimeoutSeconds).To(Equal(int32(2)))
			Expect(container.LivenessProbe.FailureThreshold).To(Equal(int32(3)))
			Expect(container.VolumeMounts).To(HaveLen(1))
			Expect(container.VolumeMounts[0].Name).To(Equal("data"))
			Expect(container.VolumeMounts[0].MountPath).To(Equal(
				"/var/lib/mysql",
			))

			By("Reconciling again without changing the desired state")
			By("Checking the Service created by the reconciler")
			service := &corev1.Service{}
			Expect(k8sClient.Get(
				ctx,
				typeNamespacedName,
				service,
			)).To(Succeed())

			Expect(service.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
			Expect(service.Spec.Selector).To(Equal(map[string]string{
				"app.kubernetes.io/name":     mysqlName,
				"app.kubernetes.io/instance": resourceName,
			}))
			Expect(service.Spec.Ports).To(HaveLen(1))
			Expect(service.Spec.Ports[0].Name).To(Equal(mysqlName))
			Expect(service.Spec.Ports[0].Protocol).To(Equal(corev1.ProtocolTCP))
			Expect(service.Spec.Ports[0].Port).To(Equal(int32(3306)))
			Expect(service.Spec.Ports[0].TargetPort.IntVal).To(Equal(int32(3306)))
			Expect(metav1.IsControlledBy(service, updated)).To(BeTrue())

			serviceResourceVersion := service.ResourceVersion

			deploymentResourceVersion := deployment.ResourceVersion
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			stableDeployment := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, stableDeployment)).To(Succeed())
			Expect(stableDeployment.ResourceVersion).To(Equal(deploymentResourceVersion))

			By("Checking that the Service remains unchanged")
			stableService := &corev1.Service{}
			Expect(k8sClient.Get(
				ctx,
				typeNamespacedName,
				stableService,
			)).To(Succeed())
			Expect(stableService.ResourceVersion).To(Equal(serviceResourceVersion))
			Expect(stableService.Spec.ClusterIP).To(Equal(service.Spec.ClusterIP))

			By("Checking that a second reconcile preserves the root password")
			stableSecret := &corev1.Secret{}
			Expect(k8sClient.Get(
				ctx,
				secretNamespacedName,
				stableSecret,
			)).To(Succeed())
			Expect(stableSecret.Data["root-password"]).To(Equal(originalPassword))
		})
	})
})
