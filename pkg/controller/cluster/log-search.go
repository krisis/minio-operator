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

package cluster

import (
	"context"
	"database/sql"
	"fmt"

	// This is required to load the postgres driver for database/sql
	_ "github.com/lib/pq"
	miniov1 "github.com/minio/operator/pkg/apis/minio.min.io/v1"
	"github.com/minio/operator/pkg/resources/services"
	corev1 "k8s.io/api/core/v1"
)

func prepareLogDB(ctx context.Context, tenant *miniov1.Tenant, secret *corev1.Secret) error {
	pgPasswd := string(secret.Data[miniov1.LogPgPassKey])
	dbAddr := services.GetLogSearchDBAddr(tenant)
	connStr := fmt.Sprintf("postgres://%s:%s@%s?sslmode=disable", miniov1.LogPgUser, pgPasswd, dbAddr)
	fmt.Println(pgPasswd, connStr)
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return err
	}

	query := fmt.Sprintf("CREATE DATABASE %s", miniov1.LogAuditDB)
	_, err = db.ExecContext(ctx, query)
	return err
}

type auditWebhookConfig struct {
	target string
	args   string
}

func newAuditWebhookConfig(tenant *miniov1.Tenant, secret *corev1.Secret) auditWebhookConfig {
	auditToken := string(secret.Data[miniov1.LogAuditTokenKey])
	whTarget := fmt.Sprintf("audit_webhook:%s", tenant.LogSearchAPIDeploymentName())

	// audit_webhook enable=off endpoint= auth_token=
	whArgs := fmt.Sprintf("%s auth_token=\"%s\" endpoint=\"%s\"", whTarget, auditToken, services.GetLogSearchAPIAddr(tenant))
	return auditWebhookConfig{
		target: whTarget,
		args:   whArgs,
	}
}
