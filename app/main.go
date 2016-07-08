// Copyright (c) 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"cocoon/commands"
	"cocoon/db"

	"io/ioutil"

	"strings"

	"appengine"
	"appengine/user"
)

func init() {
	registerRPC("/api/create-agent", commands.CreateAgent)
	registerRPC("/api/authorize-agent", commands.AuthorizeAgent)
	registerRPC("/api/get-status", commands.GetStatus)
	registerRPC("/api/refresh-github-commits", commands.RefreshGithubCommits)
	registerRPC("/api/reserve-task", commands.ReserveTask)
	registerRPC("/api/update-task-status", commands.UpdateTaskStatus)
}

func registerRPC(path string, handler func(cocoon *db.Cocoon, inputJSON []byte) (interface{}, error)) {
	http.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		ctx := appengine.NewContext(r)
		cocoon := db.NewCocoon(ctx)
		err := checkAuthentication(cocoon, r)

		if err != nil {
			serveUnauthorizedAccess(w, err)
			return
		}

		bytes, err := ioutil.ReadAll(r.Body)
		if err != nil {
			serveError(w, err)
			return
		}

		response, err := handler(cocoon, bytes)
		if err != nil {
			serveError(w, err)
			return
		}

		outputData, err := json.Marshal(response)
		if err != nil {
			serveError(w, err)
			return
		}

		w.Write(outputData)
	})
}

func authenticateAgent(ctx appengine.Context, agentID string, agentAuthToken string) (*db.Agent, error) {
	cocoon := db.NewCocoon(ctx)
	agent, err := cocoon.GetAgentByAuthToken(agentID, agentAuthToken)

	if err != nil {
		return nil, err
	}

	return agent, nil
}

func serveError(w http.ResponseWriter, err error) {
	http.Error(w, fmt.Sprintf("%v", err), http.StatusInternalServerError)
}

func serveUnauthorizedAccess(w http.ResponseWriter, err error) {
	http.Error(w, fmt.Sprintf("Authentication/authorization error: %v", err), http.StatusUnauthorized)
}

func checkAuthentication(cocoon *db.Cocoon, r *http.Request) error {
	agentAuthToken := r.Header.Get("Agent-Auth-Token")
	isCron := r.Header.Get("X-Appengine-Cron") == "true"
	if agentAuthToken != "" {
		// Authenticate as an agent
		agentID := r.Header.Get("Agent-ID")
		agent, err := authenticateAgent(cocoon.Ctx, agentID, agentAuthToken)

		if err != nil {
			return fmt.Errorf("Invalid agent: %v", agentID)
		}

		cocoon.CurrentAgent = agent
		return nil
	} else if appengine.IsDevAppServer() || isCron {
		// Authenticate on dev server and cron requests
		return nil
	} else {
		// Authenticate as user
		user, err := user.CurrentOAuth(cocoon.Ctx, "https://www.googleapis.com/auth/userinfo.email")

		if err != nil {
			return fmt.Errorf("User not signed in: %v", err)
		}

		if user == nil {
			return fmt.Errorf("User not signed in for unknown reason")
		}

		if !strings.HasSuffix(user.Email, "@google.com") {
			return fmt.Errorf("Only @google.com users are authorized")
		}

		return nil
	}
}
