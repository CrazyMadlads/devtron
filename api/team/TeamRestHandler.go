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

package team

import (
	"encoding/json"
	"fmt"
	"github.com/devtron-labs/devtron/api/restHandler/common"
	"github.com/devtron-labs/devtron/pkg/team"
	"github.com/devtron-labs/devtron/pkg/user"
	"github.com/devtron-labs/devtron/pkg/user/casbin"
	"github.com/gorilla/mux"
	"go.uber.org/zap"
	"gopkg.in/go-playground/validator.v9"
	"net/http"
	"strconv"
	"strings"
)

type TeamRestHandler interface {
	SaveTeam(w http.ResponseWriter, r *http.Request)
	FetchAll(w http.ResponseWriter, r *http.Request)
	FetchOne(w http.ResponseWriter, r *http.Request)
	UpdateTeam(w http.ResponseWriter, r *http.Request)

	FetchForAutocomplete(w http.ResponseWriter, r *http.Request)
}

type TeamRestHandlerImpl struct {
	logger          *zap.SugaredLogger
	teamService     team.TeamService
	userService     user.UserService
	validator       *validator.Validate
	enforcer        casbin.Enforcer
	userAuthService user.UserAuthService
}

func NewTeamRestHandlerImpl(logger *zap.SugaredLogger,
	teamService team.TeamService,
	userService user.UserService,
	enforcer casbin.Enforcer,
	validator *validator.Validate, userAuthService user.UserAuthService) *TeamRestHandlerImpl {
	return &TeamRestHandlerImpl{
		logger:          logger,
		teamService:     teamService,
		userService:     userService,
		validator:       validator,
		enforcer:        enforcer,
		userAuthService: userAuthService,
	}
}

func (impl TeamRestHandlerImpl) SaveTeam(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	userId, err := impl.userService.GetLoggedInUser(r)
	if userId == 0 || err != nil {
		common.WriteJsonResp(w, err, "Unauthorized User", http.StatusUnauthorized)
		return
	}
	var bean team.TeamRequest
	err = decoder.Decode(&bean)
	if err != nil {
		impl.logger.Errorw("request err, SaveTeam", "err", err, "payload", bean)
		common.WriteJsonResp(w, err, nil, http.StatusBadRequest)
		return
	}
	bean.UserId = userId
	impl.logger.Infow("request payload, SaveTeam", "payload", bean)
	err = impl.validator.Struct(bean)
	if err != nil {
		impl.logger.Errorw("validation err, SaveTeam", "err", err, "payload", bean)
		common.WriteJsonResp(w, err, nil, http.StatusBadRequest)
		return
	}
	token := r.Header.Get("token")
	if ok := impl.enforcer.Enforce(token, casbin.ResourceTeam, casbin.ActionCreate, "*"); !ok {
		common.WriteJsonResp(w, fmt.Errorf("unauthorized user"), "Unauthorized User", http.StatusForbidden)
		return
	}

	res, err := impl.teamService.Create(&bean)
	if err != nil {
		impl.logger.Errorw("service err, SaveTeam", "err", err, "payload", bean)
		common.WriteJsonResp(w, err, nil, http.StatusInternalServerError)
		return
	}
	common.WriteJsonResp(w, err, res, http.StatusOK)
}

func (impl TeamRestHandlerImpl) FetchAll(w http.ResponseWriter, r *http.Request) {
	token := r.Header.Get("token")
	res, err := impl.teamService.FetchAllActive()
	if err != nil {
		impl.logger.Errorw("service err, FetchAllActive", "err", err)
		common.WriteJsonResp(w, err, nil, http.StatusInternalServerError)
		return
	}
	// RBAC enforcer applying
	var result []team.TeamRequest
	for _, item := range res {
		if ok := impl.enforcer.Enforce(token, casbin.ResourceTeam, casbin.ActionGet, strings.ToLower(item.Name)); ok {
			result = append(result, item)
		}
	}
	//RBAC enforcer Ends

	common.WriteJsonResp(w, err, result, http.StatusOK)
}

func (impl TeamRestHandlerImpl) FetchOne(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	id := params["id"]
	idi, err := strconv.Atoi(id)
	if err != nil {
		impl.logger.Errorw("request err, FetchOne", "err", err, "id", id)
		common.WriteJsonResp(w, err, nil, http.StatusBadRequest)
		return
	}

	res, err := impl.teamService.FetchOne(idi)
	if err != nil {
		impl.logger.Errorw("service err, FetchOne", "err", err, "id", idi)
		common.WriteJsonResp(w, err, nil, http.StatusInternalServerError)
		return
	}

	token := r.Header.Get("token")
	if ok := impl.enforcer.Enforce(token, casbin.ResourceTeam, casbin.ActionGet, strings.ToLower(res.Name)); !ok {
		common.WriteJsonResp(w, err, "Unauthorized User", http.StatusForbidden)
		return
	}

	common.WriteJsonResp(w, err, res, http.StatusOK)
}

func (impl TeamRestHandlerImpl) UpdateTeam(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	userId, err := impl.userService.GetLoggedInUser(r)
	if userId == 0 || err != nil {
		common.WriteJsonResp(w, err, "Unauthorized User", http.StatusUnauthorized)
		return
	}
	var bean team.TeamRequest
	err = decoder.Decode(&bean)
	if err != nil {
		impl.logger.Errorw("request err, UpdateTeam", "err", err, "bean", bean)
		common.WriteJsonResp(w, err, nil, http.StatusBadRequest)
		return
	}
	bean.UserId = userId
	impl.logger.Infow("request payload, UpdateTeam", "err", err, "bean", bean)
	err = impl.validator.Struct(bean)
	if err != nil {
		impl.logger.Errorw("validation err, UpdateTeam", "err", err, "bean", bean)
		common.WriteJsonResp(w, err, nil, http.StatusBadRequest)
		return
	}
	token := r.Header.Get("token")
	if ok := impl.enforcer.Enforce(token, casbin.ResourceTeam, casbin.ActionUpdate, strings.ToLower(bean.Name)); !ok {
		common.WriteJsonResp(w, fmt.Errorf("unauthorized user"), "Unauthorized User", http.StatusForbidden)
		return
	}
	res, err := impl.teamService.Update(&bean)
	if err != nil {
		impl.logger.Errorw("service err, UpdateTeam", "err", err, "bean", bean)
		common.WriteJsonResp(w, err, nil, http.StatusInternalServerError)
		return
	}
	common.WriteJsonResp(w, err, res, http.StatusOK)
}

func (impl TeamRestHandlerImpl) FetchForAutocomplete(w http.ResponseWriter, r *http.Request) {
	userId, err := impl.userService.GetLoggedInUser(r)
	if userId == 0 || err != nil {
		common.WriteJsonResp(w, err, "Unauthorized User", http.StatusUnauthorized)
		return
	}
	teams, err := impl.teamService.FetchForAutocomplete()
	if err != nil {
		impl.logger.Errorw("service err, FetchForAutocomplete", "err", err)
		common.WriteJsonResp(w, err, nil, http.StatusInternalServerError)
		return
	}
	token := r.Header.Get("token")
	// RBAC enforcer applying
	var grantedTeams []team.TeamRequest
	for _, item := range teams {
		if ok := impl.enforcer.Enforce(token, casbin.ResourceTeam, casbin.ActionGet, strings.ToLower(item.Name)); ok {
			grantedTeams = append(grantedTeams, item)
		}
	}
	//RBAC enforcer Ends
	if len(grantedTeams) == 0 {
		grantedTeams = make([]team.TeamRequest, 0)
	}
	common.WriteJsonResp(w, err, grantedTeams, http.StatusOK)
}
