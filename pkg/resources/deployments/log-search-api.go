/*
 * Copyright (C) 2020, MinIO, Inc.
 *
 * This code is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License, version 3,
 * as published by the Free Software Foundation.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License, version 3,
 * along with this program.  If not, see <http://www.gnu.org/licenses/>
 *
 */

package deployments

import (
	"fmt"

	miniov1 "github.com/minio/operator/pkg/apis/minio.min.io/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Adds required log-search-api environment variables
func logSearchAPIEnvVars(t *miniov1.Tenant) []corev1.EnvVar {
	var retentionPeriod int
	if t.Spec.Log.Audit.RetentionPeriod != nil {
		retentionPeriod = *t.Spec.Log.Audit.RetentionPeriod
	}
	return []corev1.EnvVar{
		{
			Name:  miniov1.LogRetentionPeriodKey,
			Value: fmt.Sprintf("%d", retentionPeriod),
		},
		{
			Name: miniov1.LogPgConnStr,
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: t.LogSecretName(),
					},
					Key: miniov1.LogPgConnStr,
				},
			},
		},
		{
			Name: miniov1.LogAuditTokenKey,
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: t.LogSecretName(),
					},
					Key: miniov1.LogAuditTokenKey,
				},
			},
		},
	}
}

func logSearchAPIContainer(t *miniov1.Tenant) corev1.Container {
	return corev1.Container{
		Name:  miniov1.LogSearchAPIContainerName,
		Image: miniov1.LogSearchAPIImage,
		Ports: []corev1.ContainerPort{
			{
				ContainerPort: miniov1.LogSearchAPIPort,
			},
		},
		ImagePullPolicy: t.Spec.ImagePullPolicy,
		Env:             logSearchAPIEnvVars(t),
	}
}

func logSearchAPIMeta(t *miniov1.Tenant) metav1.ObjectMeta {
	meta := metav1.ObjectMeta{}
	meta.Labels = make(map[string]string)
	for k, v := range t.LogSearchAPIPodLabels() {
		meta.Labels[k] = v
	}
	return meta
}

// logSearchAPISelector Returns the Log search API Pod selector
func logSearchAPISelector(t *miniov1.Tenant) *metav1.LabelSelector {
	return &metav1.LabelSelector{
		MatchLabels: t.LogSearchAPIPodLabels(),
	}
}

// NewForLogSearchAPI returns k8s deployment object for Log Search API server
func NewForLogSearchAPI(t *miniov1.Tenant) *appsv1.Deployment {
	var replicas int32 = 1
	d := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       t.Namespace,
			Name:            t.LogSearchAPIDeploymentName(),
			OwnerReferences: t.OwnerRef(),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas, // TODO: decide how many instances we need
			Selector: logSearchAPISelector(t),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: logSearchAPIMeta(t),
				Spec: corev1.PodSpec{
					ServiceAccountName: t.Spec.ServiceAccountName,
					Containers:         []corev1.Container{logSearchAPIContainer(t)},
					RestartPolicy:      corev1.RestartPolicyAlways,
				},
			},
		},
	}

	if t.Spec.ImagePullSecret.Name != "" {
		d.Spec.Template.Spec.ImagePullSecrets = []corev1.LocalObjectReference{t.Spec.ImagePullSecret}
	}

	return d
}
