/*
 * Cherry - An OpenFlow Controller
 *
 * Copyright (C) 2015-2019 Samjung Data Service, Inc. All rights reserved.
 *
 *  Kitae Kim <superkkt@sds.co.kr>
 *  Donam Kim <donam.kim@sds.co.kr>
 *  Jooyoung Kang <jooyoung.kang@sds.co.kr>
 *  Changjin Choi <ccj9707@sds.co.kr>
 *
 * This program is free software; you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation; either version 2 of the License, or
 * any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License along
 * with this program; if not, write to the Free Software Foundation, Inc.,
 * 51 Franklin Street, Fifth Floor, Boston, MA 02110-1301 USA.
 */

package ui

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/superkkt/cherry/api"

	"github.com/ant0ine/go-json-rest/rest"
	"github.com/davecgh/go-spew/spew"
)

type Host struct {
	ID          uint64
	IP          string // FIXME: Use a native type.
	Port        string
	Group       string
	MAC         string // FIXME: Use a native type.
	Description string
	Enabled     bool
	Stale       bool
	Timestamp   time.Time
}

func (r *Host) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		ID          uint64 `json:"id"`
		IP          string `json:"ip"`
		Port        string `json:"port"`
		Group       string `json:"group"`
		MAC         string `json:"mac"`
		Description string `json:"description"`
		Enabled     bool   `json:"enabled"`
		Stale       bool   `json:"stale"`
		Timestamp   int64  `json:"timestamp"`
	}{
		ID:          r.ID,
		IP:          r.IP,
		Port:        r.Port,
		Group:       r.Group,
		MAC:         r.MAC,
		Description: r.Description,
		Enabled:     r.Enabled,
		Stale:       r.Stale,
		Timestamp:   r.Timestamp.Unix(),
	})
}

func (r *API) addHost(w rest.ResponseWriter, req *rest.Request) {
	p := new(addHostParam)
	if err := req.DecodeJsonPayload(p); err != nil {
		logger.Warningf("failed to decode params: %v", err)
		w.WriteJson(&api.Response{Status: api.StatusInvalidParameter, Message: err.Error()})
		return
	}
	logger.Debugf("addHost request from %v: %v", req.RemoteAddr, spew.Sdump(p))

	if _, ok := r.session.Get(p.SessionID); ok == false {
		logger.Warningf("unknown session id: %v", p.SessionID)
		w.WriteJson(&api.Response{Status: api.StatusUnknownSession, Message: fmt.Sprintf("unknown session id: %v", p.SessionID)})
		return
	}

	host, duplicated, err := r.DB.AddHost(p.IPID, p.GroupID, p.MAC, p.Description)
	if err != nil {
		logger.Errorf("failed to add a new host: %v", err)
		w.WriteJson(&api.Response{Status: api.StatusInternalServerError, Message: err.Error()})
		return
	}
	if duplicated {
		logger.Infof("duplicated host: ip_id=%v", p.IPID)
		w.WriteJson(&api.Response{Status: api.StatusDuplicated, Message: fmt.Sprintf("duplicated host: ip_id=%v", p.IPID)})
		return
	}
	logger.Debugf("added host info: %v", spew.Sdump(host))

	for _, v := range host {
		if err := r.announce(v.IP, v.MAC); err != nil {
			// Ignore this error.
			logger.Errorf("failed to send ARP announcement: %v", err)
		}
	}

	w.WriteJson(&api.Response{Status: api.StatusOkay, Data: host})
}

type addHostParam struct {
	SessionID   string
	IPID        []uint64
	GroupID     *uint64
	MAC         net.HardwareAddr
	Description string
}

func (r *addHostParam) UnmarshalJSON(data []byte) error {
	v := struct {
		SessionID   string   `json:"session_id"`
		IPID        []uint64 `json:"ip_id"`
		GroupID     *uint64  `json:"group_id"`
		MAC         string   `json:"mac"`
		Description string   `json:"description"`
	}{}
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}

	if len(v.SessionID) != 64 {
		return errors.New("invalid session id")
	}
	if len(v.IPID) == 0 {
		return errors.New("empty ip id")
	}
	if len(v.IPID) > 10 {
		return errors.New("too many ip ids")
	}
	for _, i := range v.IPID {
		if i == 0 {
			return errors.New("invalid ip id")
		}
	}
	if len(v.Description) > 255 {
		return errors.New("too long description")
	}
	mac, err := net.ParseMAC(v.MAC)
	if err != nil {
		return err
	}

	r.SessionID = v.SessionID
	r.IPID = v.IPID
	r.GroupID = v.GroupID
	r.MAC = mac
	r.Description = v.Description

	return nil
}

func (r *API) updateHost(w rest.ResponseWriter, req *rest.Request) {
	p := new(updateHostParam)
	if err := req.DecodeJsonPayload(p); err != nil {
		logger.Warningf("failed to decode params: %v", err)
		w.WriteJson(&api.Response{Status: api.StatusInvalidParameter, Message: err.Error()})
		return
	}
	logger.Debugf("updateHost request from %v: %v", req.RemoteAddr, spew.Sdump(p))

	if _, ok := r.session.Get(p.SessionID); ok == false {
		logger.Warningf("unknown session id: %v", p.SessionID)
		w.WriteJson(&api.Response{Status: api.StatusUnknownSession, Message: fmt.Sprintf("unknown session id: %v", p.SessionID)})
		return
	}

	old, err := r.DB.Host(p.ID)
	if err != nil {
		logger.Errorf("failed to query the host: %v", err)
		w.WriteJson(&api.Response{Status: api.StatusInternalServerError, Message: err.Error()})
		return
	}
	if old == nil {
		logger.Infof("not found host to update: %v", p.ID)
		w.WriteJson(&api.Response{Status: api.StatusNotFound, Message: fmt.Sprintf("not found host to update: %v", p.ID)})
		return
	}
	if old.Enabled == false {
		logger.Infof("unable to update blocked host: %v", p.ID)
		w.WriteJson(&api.Response{Status: api.StatusBlockedHost, Message: fmt.Sprintf("unable to update blocked host: %v", p.ID)})
		return
	}

	new, duplicated, err := r.DB.UpdateHost(p.ID, p.IPID, p.GroupID, p.MAC, p.Description)
	if err != nil {
		logger.Errorf("failed to update host info: %v", err)
		w.WriteJson(&api.Response{Status: api.StatusInternalServerError, Message: err.Error()})
		return
	}
	if duplicated {
		logger.Infof("duplicated host: ip_id=%v", p.IPID)
		w.WriteJson(&api.Response{Status: api.StatusDuplicated, Message: fmt.Sprintf("duplicated host: ip_id=%v", p.IPID)})
		return
	}
	logger.Debugf("updated host info: %v", spew.Sdump(new))

	if err := r.announce(old.IP, "00:00:00:00:00:00"); err != nil {
		// Ignore this error.
		logger.Errorf("failed to send ARP announcement: %v", err)
	}
	if err := r.announce(new.IP, new.MAC); err != nil {
		// Ignore this error.
		logger.Errorf("failed to send ARP announcement: %v", err)
	}

	w.WriteJson(&api.Response{Status: api.StatusOkay, Data: new})
}

type updateHostParam struct {
	SessionID   string
	ID          uint64
	IPID        uint64
	GroupID     *uint64
	MAC         net.HardwareAddr
	Description string
}

func (r *updateHostParam) UnmarshalJSON(data []byte) error {
	v := struct {
		SessionID   string  `json:"session_id"`
		ID          uint64  `json:"id"`
		IPID        uint64  `json:"ip_id"`
		GroupID     *uint64 `json:"group_id"`
		MAC         string  `json:"mac"`
		Description string  `json:"description"`
	}{}
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}

	if len(v.SessionID) != 64 {
		return errors.New("invalid session id")
	}
	if v.ID == 0 {
		return errors.New("invalid host id")
	}
	if v.IPID == 0 {
		return errors.New("invalid ip id")
	}
	if len(v.Description) > 255 {
		return errors.New("too long description")
	}
	mac, err := net.ParseMAC(v.MAC)
	if err != nil {
		return err
	}

	r.SessionID = v.SessionID
	r.ID = v.ID
	r.IPID = v.IPID
	r.GroupID = v.GroupID
	r.MAC = mac
	r.Description = v.Description

	return nil
}

func (r *API) activateHost(w rest.ResponseWriter, req *rest.Request) {
	p := new(activateHostParam)
	if err := req.DecodeJsonPayload(p); err != nil {
		logger.Warningf("failed to decode params: %v", err)
		w.WriteJson(&api.Response{Status: api.StatusInvalidParameter, Message: err.Error()})
		return
	}
	logger.Debugf("activateHost request from %v: %v", req.RemoteAddr, spew.Sdump(p))

	if _, ok := r.session.Get(p.SessionID); ok == false {
		logger.Warningf("unknown session id: %v", p.SessionID)
		w.WriteJson(&api.Response{Status: api.StatusUnknownSession, Message: fmt.Sprintf("unknown session id: %v", p.SessionID)})
		return
	}

	host, err := r.DB.ActivateHost(p.ID)
	if err != nil {
		logger.Errorf("failed to activate a host: %v", err)
		w.WriteJson(&api.Response{Status: api.StatusInternalServerError, Message: err.Error()})
		return
	}
	if host == nil {
		logger.Infof("not found host to activate: %v", p.ID)
		w.WriteJson(&api.Response{Status: api.StatusNotFound, Message: fmt.Sprintf("not found host to activate: %v", p.ID)})
		return
	}
	logger.Debugf("activated host info: %v", spew.Sdump(host))

	if err := r.announce(host.IP, host.MAC); err != nil {
		// Ignore this error.
		logger.Errorf("failed to send ARP announcement: %v", err)
	}

	w.WriteJson(&api.Response{Status: api.StatusOkay})
}

type activateHostParam struct {
	SessionID string
	ID        uint64
}

func (r *activateHostParam) UnmarshalJSON(data []byte) error {
	v := struct {
		SessionID string `json:"session_id"`
		ID        uint64 `json:"id"`
	}{}
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*r = activateHostParam(v)

	return r.validate()
}

func (r *activateHostParam) validate() error {
	if len(r.SessionID) != 64 {
		return errors.New("invalid session id")
	}
	if r.ID == 0 {
		return errors.New("invalid switch id")
	}

	return nil
}

func (r *API) deactivateHost(w rest.ResponseWriter, req *rest.Request) {
	p := new(deactivateHostParam)
	if err := req.DecodeJsonPayload(p); err != nil {
		logger.Warningf("failed to decode params: %v", err)
		w.WriteJson(&api.Response{Status: api.StatusInvalidParameter, Message: err.Error()})
		return
	}
	logger.Debugf("deactivateHost request from %v: %v", req.RemoteAddr, spew.Sdump(p))

	if _, ok := r.session.Get(p.SessionID); ok == false {
		logger.Warningf("unknown session id: %v", p.SessionID)
		w.WriteJson(&api.Response{Status: api.StatusUnknownSession, Message: fmt.Sprintf("unknown session id: %v", p.SessionID)})
		return
	}

	host, err := r.DB.DeactivateHost(p.ID)
	if err != nil {
		logger.Errorf("failed to deactivate a host: %v", err)
		w.WriteJson(&api.Response{Status: api.StatusInternalServerError, Message: err.Error()})
		return
	}
	if host == nil {
		logger.Infof("not found host to deactivate: %v", p.ID)
		w.WriteJson(&api.Response{Status: api.StatusNotFound, Message: fmt.Sprintf("not found host to deactivate: %v", p.ID)})
		return
	}
	logger.Debugf("deactivated host info: %v", spew.Sdump(host))

	if err := r.announce(host.IP, "00:00:00:00:00:00"); err != nil {
		// Ignore this error.
		logger.Errorf("failed to send ARP announcement: %v", err)
	}

	w.WriteJson(&api.Response{Status: api.StatusOkay})
}

type deactivateHostParam struct {
	SessionID string
	ID        uint64
}

func (r *deactivateHostParam) UnmarshalJSON(data []byte) error {
	v := struct {
		SessionID string `json:"session_id"`
		ID        uint64 `json:"id"`
	}{}
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*r = deactivateHostParam(v)

	return r.validate()
}

func (r *deactivateHostParam) validate() error {
	if len(r.SessionID) != 64 {
		return errors.New("invalid session id")
	}
	if r.ID == 0 {
		return errors.New("invalid switch id")
	}

	return nil
}

func (r *API) removeHost(w rest.ResponseWriter, req *rest.Request) {
	p := new(removeHostParam)
	if err := req.DecodeJsonPayload(p); err != nil {
		logger.Warningf("failed to decode params: %v", err)
		w.WriteJson(&api.Response{Status: api.StatusInvalidParameter, Message: err.Error()})
		return
	}
	logger.Debugf("removeHost request from %v: %v", req.RemoteAddr, spew.Sdump(p))

	if _, ok := r.session.Get(p.SessionID); ok == false {
		logger.Warningf("unknown session id: %v", p.SessionID)
		w.WriteJson(&api.Response{Status: api.StatusUnknownSession, Message: fmt.Sprintf("unknown session id: %v", p.SessionID)})
		return
	}

	host, err := r.DB.RemoveHost(p.ID)
	if err != nil {
		logger.Errorf("failed to remove host info: %v", err)
		w.WriteJson(&api.Response{Status: api.StatusInternalServerError, Message: err.Error()})
		return
	}
	if host == nil {
		logger.Infof("not found host to remove: %v", p.ID)
		w.WriteJson(&api.Response{Status: api.StatusNotFound, Message: fmt.Sprintf("not found host to remove: %v", p.ID)})
		return
	}
	logger.Debugf("removed host info: %v", spew.Sdump(host))

	if err := r.announce(host.IP, "00:00:00:00:00:00"); err != nil {
		// Ignore this error.
		logger.Errorf("failed to send ARP announcement: %v", err)
	}

	w.WriteJson(&api.Response{Status: api.StatusOkay})
}

type removeHostParam struct {
	SessionID string
	ID        uint64
}

func (r *removeHostParam) UnmarshalJSON(data []byte) error {
	v := struct {
		SessionID string `json:"session_id"`
		ID        uint64 `json:"id"`
	}{}
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*r = removeHostParam(v)

	return r.validate()
}

func (r *removeHostParam) validate() error {
	if len(r.SessionID) != 64 {
		return errors.New("invalid session id")
	}
	if r.ID == 0 {
		return errors.New("invalid switch id")
	}

	return nil
}
