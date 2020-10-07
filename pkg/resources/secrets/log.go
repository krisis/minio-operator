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

package secrets

import (
	"fmt"

	miniov1 "github.com/minio/operator/pkg/apis/minio.min.io/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LogSecret returns a k8s secret object with postgres password
func LogSecret(t *miniov1.Tenant) *corev1.Secret {
	dbAddr := ""
	pgConnStr := fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", miniov1.LogPgUser, "example", dbAddr, miniov1.LogAuditDB)
	return &corev1.Secret{
		Type: "Opaque",
		ObjectMeta: metav1.ObjectMeta{
			Name:            t.LogSecretName(),
			Namespace:       t.Namespace,
			OwnerReferences: t.OwnerRef(),
		},
		Data: map[string][]byte{
			miniov1.LogPgPassKey:     []byte("example"),     // FIXME(kp): Generate a strong enough random password,
			miniov1.LogAuditTokenKey: []byte("audit_token"), //FIXME(kp): Generate a strong enough random token
			miniov1.LogPgConnStr:     []byte(pgConnStr),
		},
	}
}
