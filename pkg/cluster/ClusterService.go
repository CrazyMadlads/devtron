/*
 * Copyright (c) 2020 Devtron Labs
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

package cluster

import (
	"context"
	"fmt"
	"github.com/devtron-labs/devtron/internal/constants"
	"github.com/devtron-labs/devtron/internal/util"
	"github.com/devtron-labs/devtron/pkg/cluster/repository"
	"github.com/go-pg/pg"
	"go.uber.org/zap"
	"io/ioutil"
	"os"
	"time"
)

type ClusterBean struct {
	Id                      int                        `json:"id,omitempty" validate:"number"`
	ClusterName             string                     `json:"cluster_name,omitempty" validate:"required"`
	ServerUrl               string                     `json:"server_url,omitempty" validate:"url,required"`
	PrometheusUrl           string                     `json:"prometheus_url,omitempty" validate:"validate-non-empty-url"`
	Active                  bool                       `json:"active"`
	Config                  map[string]string          `json:"config,omitempty" validate:"required"`
	PrometheusAuth          *PrometheusAuth            `json:"prometheusAuth,omitempty"`
	DefaultClusterComponent []*DefaultClusterComponent `json:"defaultClusterComponent"`
	AgentInstallationStage  int                        `json:"agentInstallationStage,notnull"` // -1=external, 0=not triggered, 1=progressing, 2=success, 3=fails
	K8sVersion              string                     `json:"k8sVersion"`
}

type PrometheusAuth struct {
	UserName      string `json:"userName,omitempty"`
	Password      string `json:"password,omitempty"`
	TlsClientCert string `json:"tlsClientCert,omitempty"`
	TlsClientKey  string `json:"tlsClientKey,omitempty"`
}

type DefaultClusterComponent struct {
	ComponentName  string `json:"name"`
	AppId          int    `json:"appId"`
	InstalledAppId int    `json:"installedAppId,omitempty"`
	EnvId          int    `json:"envId"`
	EnvName        string `json:"envName"`
	Status         string `json:"status"`
}

type ClusterService interface {
	Save(parent context.Context, bean *ClusterBean, userId int32) (*ClusterBean, error)
	FindOne(clusterName string) (*ClusterBean, error)
	FindOneActive(clusterName string) (*ClusterBean, error)
	FindAll() ([]*ClusterBean, error)
	FindAllActive() ([]ClusterBean, error)

	FindById(id int) (*ClusterBean, error)
	FindByIds(id []int) ([]ClusterBean, error)
	Update(ctx context.Context, bean *ClusterBean, userId int32) (*ClusterBean, error)
	Delete(bean *ClusterBean, userId int32) error

	FindAllForAutoComplete() ([]ClusterBean, error)
	CreateGrafanaDataSource(clusterBean *ClusterBean, env *repository.Environment) (int, error)
	GetClusterConfig(cluster *ClusterBean) (*util.ClusterConfig, error)
}

type ClusterServiceImpl struct {
	clusterRepository repository.ClusterRepository
	logger            *zap.SugaredLogger
	K8sUtil           *util.K8sUtil
}

func NewClusterServiceImpl(repository repository.ClusterRepository, logger *zap.SugaredLogger,
	K8sUtil *util.K8sUtil) *ClusterServiceImpl {
	return &ClusterServiceImpl{
		clusterRepository: repository,
		logger:            logger,
		K8sUtil:           K8sUtil,
	}
}

const ClusterName = "default_cluster"
const TokenFilePath = "/var/run/secrets/kubernetes.io/serviceaccount/token"

func (impl *ClusterServiceImpl) GetClusterConfig(cluster *ClusterBean) (*util.ClusterConfig, error) {
	host := cluster.ServerUrl
	configMap := cluster.Config
	bearerToken := configMap["bearer_token"]
	if cluster.Id == 1 && cluster.ClusterName == ClusterName {
		if _, err := os.Stat(TokenFilePath); os.IsNotExist(err) {
			impl.logger.Errorw("no directory or file exists", "TOKEN_FILE_PATH", TokenFilePath, "err", err)
			return nil, err
		} else {
			content, err := ioutil.ReadFile(TokenFilePath)
			if err != nil {
				impl.logger.Errorw("error on reading file", "err", err)
				return nil, err
			}
			bearerToken = string(content)
		}
	}
	clusterCfg := &util.ClusterConfig{Host: host, BearerToken: bearerToken}
	return clusterCfg, nil
}

func (impl *ClusterServiceImpl) Save(parent context.Context, bean *ClusterBean, userId int32) (*ClusterBean, error) {

	existingModel, err := impl.clusterRepository.FindOne(bean.ClusterName)
	if err != nil && err != pg.ErrNoRows {
		impl.logger.Error(err)
		return nil, err
	}
	if existingModel.Id > 0 {
		impl.logger.Errorw("error on fetching cluster, duplicate", "name", bean.ClusterName)
		return nil, fmt.Errorf("cluster already exists")
	}

	model := &repository.Cluster{
		ClusterName:        bean.ClusterName,
		Active:             bean.Active,
		ServerUrl:          bean.ServerUrl,
		Config:             bean.Config,
		PrometheusEndpoint: bean.PrometheusUrl,
	}

	if bean.PrometheusAuth != nil {
		model.PUserName = bean.PrometheusAuth.UserName
		model.PPassword = bean.PrometheusAuth.Password
		model.PTlsClientCert = bean.PrometheusAuth.TlsClientCert
		model.PTlsClientKey = bean.PrometheusAuth.TlsClientKey
	}

	model.CreatedBy = userId
	model.UpdatedBy = userId
	model.CreatedOn = time.Now()
	model.UpdatedOn = time.Now()

	cfg, err := impl.GetClusterConfig(bean)
	if err != nil {
		return nil, err
	}
	client, err := impl.K8sUtil.GetK8sDiscoveryClient(cfg)
	if err != nil {
		return nil, err
	}
	k8sServerVersion, err := client.ServerVersion()
	if err != nil {
		return nil, err
	}
	model.K8sVersion = k8sServerVersion.String()
	err = impl.clusterRepository.Save(model)
	if err != nil {
		impl.logger.Errorw("error in saving cluster in db", "err", err)
		err = &util.ApiError{
			Code:            constants.ClusterCreateDBFailed,
			InternalMessage: "cluster creation failed in db",
			UserMessage:     fmt.Sprintf("requested by %d", userId),
		}
	}
	bean.Id = model.Id
	return bean, err
}

func (impl *ClusterServiceImpl) FindOne(clusterName string) (*ClusterBean, error) {
	model, err := impl.clusterRepository.FindOne(clusterName)
	if err != nil {
		return nil, err
	}
	bean := &ClusterBean{
		Id:                     model.Id,
		ClusterName:            model.ClusterName,
		ServerUrl:              model.ServerUrl,
		PrometheusUrl:          model.PrometheusEndpoint,
		AgentInstallationStage: model.AgentInstallationStage,
		Active:                 model.Active,
		Config:                 model.Config,
		K8sVersion:             model.K8sVersion,
	}
	return bean, nil
}

func (impl *ClusterServiceImpl) FindOneActive(clusterName string) (*ClusterBean, error) {
	model, err := impl.clusterRepository.FindOneActive(clusterName)
	if err != nil {
		return nil, err
	}
	bean := &ClusterBean{
		Id:                     model.Id,
		ClusterName:            model.ClusterName,
		ServerUrl:              model.ServerUrl,
		PrometheusUrl:          model.PrometheusEndpoint,
		AgentInstallationStage: model.AgentInstallationStage,
		Active:                 model.Active,
		Config:                 model.Config,
		K8sVersion:             model.K8sVersion,
	}
	return bean, nil
}

func (impl *ClusterServiceImpl) FindAll() ([]*ClusterBean, error) {
	model, err := impl.clusterRepository.FindAllActive()
	if err != nil {
		return nil, err
	}
	var beans []*ClusterBean
	for _, m := range model {
		beans = append(beans, &ClusterBean{
			Id:                     m.Id,
			ClusterName:            m.ClusterName,
			PrometheusUrl:          m.PrometheusEndpoint,
			AgentInstallationStage: m.AgentInstallationStage,
			ServerUrl:              m.ServerUrl,
			Active:                 m.Active,
			K8sVersion:             m.K8sVersion,
		})
	}
	return beans, nil
}

func (impl *ClusterServiceImpl) FindAllActive() ([]ClusterBean, error) {
	model, err := impl.clusterRepository.FindAllActive()
	if err != nil {
		return nil, err
	}
	var beans []ClusterBean
	for _, m := range model {
		beans = append(beans, ClusterBean{
			Id:                     m.Id,
			ClusterName:            m.ClusterName,
			ServerUrl:              m.ServerUrl,
			Active:                 m.Active,
			PrometheusUrl:          m.PrometheusEndpoint,
			AgentInstallationStage: m.AgentInstallationStage,
			Config:                 m.Config,
			K8sVersion:             m.K8sVersion,
		})
	}
	return beans, nil
}

func (impl *ClusterServiceImpl) FindById(id int) (*ClusterBean, error) {
	model, err := impl.clusterRepository.FindById(id)
	if err != nil {
		return nil, err
	}
	bean := &ClusterBean{
		Id:                     model.Id,
		ClusterName:            model.ClusterName,
		ServerUrl:              model.ServerUrl,
		PrometheusUrl:          model.PrometheusEndpoint,
		AgentInstallationStage: model.AgentInstallationStage,
		Active:                 model.Active,
		Config:                 model.Config,
		K8sVersion:             model.K8sVersion,
	}
	prometheusAuth := &PrometheusAuth{
		UserName:      model.PUserName,
		Password:      model.PPassword,
		TlsClientCert: model.PTlsClientCert,
		TlsClientKey:  model.PTlsClientKey,
	}
	bean.PrometheusAuth = prometheusAuth
	return bean, nil
}

func (impl *ClusterServiceImpl) FindByIds(ids []int) ([]ClusterBean, error) {
	models, err := impl.clusterRepository.FindByIds(ids)
	if err != nil {
		return nil, err
	}
	var beans []ClusterBean

	for _, model := range models {
		beans = append(beans, ClusterBean{
			Id:                     model.Id,
			ClusterName:            model.ClusterName,
			ServerUrl:              model.ServerUrl,
			PrometheusUrl:          model.PrometheusEndpoint,
			AgentInstallationStage: model.AgentInstallationStage,
			Active:                 model.Active,
			Config:                 model.Config,
			K8sVersion:             model.K8sVersion,
		})
	}
	return beans, nil
}

func (impl *ClusterServiceImpl) Update(ctx context.Context, bean *ClusterBean, userId int32) (*ClusterBean, error) {
	model, err := impl.clusterRepository.FindById(bean.Id)
	if err != nil {
		impl.logger.Error(err)
		return nil, err
	}

	existingModel, err := impl.clusterRepository.FindOne(bean.ClusterName)
	if err != nil && err != pg.ErrNoRows {
		impl.logger.Error(err)
		return nil, err
	}
	if existingModel.Id > 0 && model.Id != existingModel.Id {
		impl.logger.Errorw("error on fetching cluster, duplicate", "name", bean.ClusterName)
		return nil, fmt.Errorf("cluster already exists")
	}

	model.ClusterName = bean.ClusterName
	model.ServerUrl = bean.ServerUrl
	model.PrometheusEndpoint = bean.PrometheusUrl

	if bean.PrometheusAuth != nil {
		model.PUserName = bean.PrometheusAuth.UserName
		model.PPassword = bean.PrometheusAuth.Password
		model.PTlsClientCert = bean.PrometheusAuth.TlsClientCert
		model.PTlsClientKey = bean.PrometheusAuth.TlsClientKey
	}

	model.Active = bean.Active
	model.Config = bean.Config
	model.UpdatedBy = userId
	model.UpdatedOn = time.Now()

	if model.K8sVersion == "" {
		cfg, err := impl.GetClusterConfig(bean)
		if err != nil {
			return nil, err
		}
		client, err := impl.K8sUtil.GetK8sDiscoveryClient(cfg)
		if err != nil {
			return nil, err
		}
		k8sServerVersion, err := client.ServerVersion()
		if err != nil {
			return nil, err
		}
		model.K8sVersion = k8sServerVersion.String()
	}
	err = impl.clusterRepository.Update(model)
	if err != nil {
		err = &util.ApiError{
			Code:            constants.ClusterUpdateDBFailed,
			InternalMessage: "cluster update failed in db",
			UserMessage:     fmt.Sprintf("requested by %d", userId),
		}
		return bean, err
	}

	bean.Id = model.Id
	return bean, err
}

func (impl *ClusterServiceImpl) Delete(bean *ClusterBean, userId int32) error {
	model, err := impl.clusterRepository.FindById(bean.Id)
	if err != nil {
		return err
	}
	return impl.clusterRepository.Delete(model)
}

func (impl *ClusterServiceImpl) FindAllForAutoComplete() ([]ClusterBean, error) {
	model, err := impl.clusterRepository.FindAll()
	if err != nil {
		return nil, err
	}
	var beans []ClusterBean
	for _, m := range model {
		beans = append(beans, ClusterBean{
			Id:          m.Id,
			ClusterName: m.ClusterName,
		})
	}
	return beans, nil
}

func (impl *ClusterServiceImpl) CreateGrafanaDataSource(clusterBean *ClusterBean, env *repository.Environment) (int, error) {
	impl.logger.Errorw("CreateGrafanaDataSource not inplementd in ClusterServiceImpl")
	return 0, fmt.Errorf("method not implemented")
}
