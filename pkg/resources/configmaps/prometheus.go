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

package configmaps

import (
	"fmt"
	"reflect"
	"time"

	"github.com/dgrijalva/jwt-go"
	jwtgo "github.com/dgrijalva/jwt-go"
	miniov1 "github.com/minio/operator/pkg/apis/minio.min.io/v1"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	defaultPrometheusJWTExpiry = 100 * 365 * 24 * time.Hour
)

type globalConfig struct {
	ScrapeInterval     time.Duration `yaml:"scrape_interval"`
	EvaluationInterval time.Duration `yaml:"evaluation_interval"`
}

type staticConfig struct {
	Targets []string `yaml:"targets"`
}

type tlsConfig struct {
	CAFile string `yaml:"ca_file"`
}

type scrapeConfig struct {
	JobName       string         `yaml:"job_name"`
	BearerToken   string         `yaml:"bearer_token"`
	MetricsPath   string         `yaml:"metrics_path"`
	Scheme        string         `yaml:"scheme"`
	TLSConfig     tlsConfig      `yaml:"tls_config"`
	StaticConfigs []staticConfig `yaml:"static_configs"`
}

type prometheusConfig struct {
	Global        globalConfig   `yaml:"global"`
	ScrapeConfigs []scrapeConfig `yaml:"scrape_configs"`
}

func genBearerToken(accessKey, secretKey string) string {
	jwt := jwtgo.NewWithClaims(jwtgo.SigningMethodHS512, jwtgo.StandardClaims{
		ExpiresAt: time.Now().UTC().Add(defaultPrometheusJWTExpiry).Unix(),
		Subject:   accessKey,
		Issuer:    "prometheus",
	})

	token, err := jwt.SignedString([]byte(secretKey))
	if err != nil {
		panic(fmt.Sprintf("jwt key generation: %v", err))
	}

	return token
}

// getMinioPodAddrs returns a list of stable minio pod addresses.
func getMinioPodAddrs(t *miniov1.Tenant) []string {
	targets := []string{}
	for _, pool := range t.Spec.Pools {
		poolName := t.PoolStatefulsetName(&pool)
		for i := 0; i < int(pool.Servers); i++ {
			target := fmt.Sprintf("%s-%d.%s.%s.svc.%s:%d", poolName, i, t.MinIOHLServiceName(), t.Namespace, miniov1.GetClusterDomain(), miniov1.MinIOPort)
			targets = append(targets, target)
		}
	}
	return targets
}

func (p *prometheusConfig) ConfigFile() string {
	d, err := yaml.Marshal(p)
	if err != nil {
		panic(fmt.Sprintf("error marshaling to yaml: %v", err))
	}

	configFileContent := fmt.Sprintf(`# This file and config-map is generated by MinIO Operator.
# DO NOT EDIT.

%s`, d)
	return configFileContent
}

func getPrometheusConfig(t *miniov1.Tenant, accessKey, secretKey string) *prometheusConfig {
	bearerToken := genBearerToken(accessKey, secretKey)
	minioTargets := getMinioPodAddrs(t)
	minioScheme := "http"
	if t.TLS() {
		minioScheme = "https"
	}

	// populate config
	promConfig := &prometheusConfig{
		Global: globalConfig{
			ScrapeInterval:     10 * time.Second,
			EvaluationInterval: 30 * time.Second,
		},
		ScrapeConfigs: []scrapeConfig{
			{
				JobName:     "minio",
				BearerToken: bearerToken,
				MetricsPath: "/minio/prometheus/metrics",
				Scheme:      minioScheme,
				TLSConfig: tlsConfig{
					CAFile: "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt",
				},
				StaticConfigs: []staticConfig{
					{
						Targets: minioTargets,
					},
				},
			},
		},
	}
	return promConfig
}

const prometheusYml = "prometheus.yml"

// fromPrometheusConfigMap parses prometheus config file from the given
// configmap and returns *prometheusConfig on success. Otherwise returns error.
func fromPrometheusConfigMap(configMap *corev1.ConfigMap) (*prometheusConfig, error) {
	configFile := configMap.Data[prometheusYml]
	var config prometheusConfig
	err := yaml.Unmarshal([]byte(configFile), &config)
	if err != nil {
		return nil, err
	}
	return &config, nil
}

// getConfigMap returns k8s config map for the given tenant
func (p *prometheusConfig) getConfigMap(tenant *miniov1.Tenant) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            tenant.PrometheusConfigMapName(),
			Namespace:       tenant.Namespace,
			OwnerReferences: tenant.OwnerRef(),
		},
		Data: map[string]string{
			prometheusYml: p.ConfigFile(),
		},
	}
}

// bearerTokenNeedsUpdate returns true if the prometheusConfig's bearer token
// can't be verified using the given secretKey
func (p *prometheusConfig) bearerTokenNeedsUpdate(secretKey string) bool {
	tokenStr := p.ScrapeConfigs[0].BearerToken
	_, err := jwtgo.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("Unexpected signing method: %v", token.Header["alg"])
		}
		return secretKey, nil
	})
	return err != nil
}

// PrometheusConfigMap returns k8s configmap containing Prometheus configuration.
func PrometheusConfigMap(tenant *miniov1.Tenant, accessKey, secretKey string) *corev1.ConfigMap {
	config := getPrometheusConfig(tenant, accessKey, secretKey)
	return config.getConfigMap(tenant)
}

// UpdatePrometheusConfigMap checks if the prometheus config map needs update
// and if so returns the updated map. Otherwise it returns nil.
func UpdatePrometheusConfigMap(t *miniov1.Tenant, accessKey, secretKey string, existing *corev1.ConfigMap) *corev1.ConfigMap {
	config := getPrometheusConfig(t, accessKey, secretKey)
	existingConfig, err := fromPrometheusConfigMap(existing)
	if err != nil {
		// needs update to recover possibly corrupt current config file
		return config.getConfigMap(t)
	}

	// Validate existing config bearer token with the secret key from
	// 'desired' tenant spec. Success indicates that the secret key hasn't
	// changed, so 'mask' the bearer token before performing equality check
	// to determine if the prometheus config requires update.
	needsUpdate := existingConfig.bearerTokenNeedsUpdate(secretKey)
	if !needsUpdate {
		config.ScrapeConfigs[0].BearerToken = ""
		existingConfig.ScrapeConfigs[0].BearerToken = ""
		if !reflect.DeepEqual(existingConfig, config) {
			needsUpdate = true
		}
	}

	if needsUpdate {
		return config.getConfigMap(t)
	}

	return nil
}
