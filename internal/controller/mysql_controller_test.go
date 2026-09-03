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
			Expect(deployment.Spec.Replicas).NotTo(BeNil())
			Expect(*deployment.Spec.Replicas).To(Equal(int32(1)))
			Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(1))
			Expect(deployment.Spec.Template.Spec.Containers[0].Image).To(Equal("mysql:8.4"))
			Expect(metav1.IsControlledBy(deployment, updated)).To(BeTrue())

			By("Reconciling again without changing the desired state")
			deploymentResourceVersion := deployment.ResourceVersion
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			stableDeployment := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, stableDeployment)).To(Succeed())
			Expect(stableDeployment.ResourceVersion).To(Equal(deploymentResourceVersion))
		})
	})
})
