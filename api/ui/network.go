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

	"github.com/superkkt/cherry/api"

	"github.com/ant0ine/go-json-rest/rest"
	"github.com/davecgh/go-spew/spew"
)

type Network struct {
	ID      uint64 `json:"id"`
	Address string `json:"address"` // FIXME: Use a native type.
	Mask    uint8  `json:"mask"`    // FIXME: Use a native type.
}

func (r *API) listNetwork(w rest.ResponseWriter, req *rest.Request) {
	p := new(listNetworkParam)
	if err := req.DecodeJsonPayload(p); err != nil {
		logger.Warningf("failed to decode params: %v", err)
		w.WriteJson(&api.Response{Status: api.StatusInvalidParameter, Message: err.Error()})
		return
	}
	logger.Debugf("listNetwork request from %v: %v", req.RemoteAddr, spew.Sdump(p))

	if _, ok := r.session.Get(p.SessionID); ok == false {
		logger.Warningf("unknown session id: %v", p.SessionID)
		w.WriteJson(&api.Response{Status: api.StatusUnknownSession, Message: fmt.Sprintf("unknown session id: %v", p.SessionID)})
		return
	}

	network, err := r.DB.Networks(p.Offset, p.Limit)
	if err != nil {
		logger.Errorf("failed to query the network list: %v", err)
		w.WriteJson(&api.Response{Status: api.StatusInternalServerError, Message: err.Error()})
		return
	}
	logger.Debugf("queried network list: %v", spew.Sdump(network))

	w.WriteJson(&api.Response{Status: api.StatusOkay, Data: network})
}

type listNetworkParam struct {
	SessionID string
	Offset    uint32
	Limit     uint8
}

func (r *listNetworkParam) UnmarshalJSON(data []byte) error {
	v := struct {
		SessionID string `json:"session_id"`
		Offset    uint32 `json:"offset"`
		Limit     uint8  `json:"limit"`
	}{}
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*r = listNetworkParam(v)

	return r.validate()
}

func (r *listNetworkParam) validate() error {
	if len(r.SessionID) != 64 {
		return errors.New("invalid session id")
	}
	if r.Limit == 0 {
		return errors.New("invalid limit")
	}

	return nil
}

func (r *API) addNetwork(w rest.ResponseWriter, req *rest.Request) {
	p := new(addNetworkParam)
	if err := req.DecodeJsonPayload(p); err != nil {
		logger.Warningf("failed to decode params: %v", err)
		w.WriteJson(&api.Response{Status: api.StatusInvalidParameter, Message: err.Error()})
		return
	}
	logger.Debugf("addNetwork request from %v: %v", req.RemoteAddr, spew.Sdump(p))

	if _, ok := r.session.Get(p.SessionID); ok == false {
		logger.Warningf("unknown session id: %v", p.SessionID)
		w.WriteJson(&api.Response{Status: api.StatusUnknownSession, Message: fmt.Sprintf("unknown session id: %v", p.SessionID)})
		return
	}

	id, duplicated, err := r.DB.AddNetwork(p.Address, p.Mask)
	if err != nil {
		logger.Errorf("failed to add a new network: %v", err)
		w.WriteJson(&api.Response{Status: api.StatusInternalServerError, Message: err.Error()})
		return
	}
	if duplicated {
		logger.Infof("duplicated network: address=%v, mask=%v", p.Address, p.Mask)
		w.WriteJson(&api.Response{Status: api.StatusDuplicated, Message: fmt.Sprintf("duplicated network: address=%v, mask=%v", p.Address, p.Mask)})
		return
	}
	logger.Debugf("added network info: %v", spew.Sdump(p))

	w.WriteJson(&api.Response{Status: api.StatusOkay, Data: id})
}

type addNetworkParam struct {
	SessionID string
	Address   net.IP
	Mask      net.IPMask
}

func (r *addNetworkParam) UnmarshalJSON(data []byte) error {
	v := struct {
		SessionID string `json:"session_id"`
		Address   string `json:"address"`
		Mask      uint8  `json:"mask"`
	}{}
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}

	if len(v.SessionID) != 64 {
		return errors.New("invalid session id")
	}
	addr := net.ParseIP(v.Address)
	if addr == nil {
		return fmt.Errorf("invalid network address: %v", v.Address)
	}
	if v.Mask < 24 || v.Mask > 30 {
		return fmt.Errorf("invalid network mask: %v", v.Mask)
	}

	r.SessionID = v.SessionID
	r.Mask = net.CIDRMask(int(v.Mask), 32)
	r.Address = addr.Mask(r.Mask)

	return nil
}

func (r *API) removeNetwork(w rest.ResponseWriter, req *rest.Request) {
	p := new(removeNetworkParam)
	if err := req.DecodeJsonPayload(p); err != nil {
		logger.Warningf("failed to decode params: %v", err)
		w.WriteJson(&api.Response{Status: api.StatusInvalidParameter, Message: err.Error()})
		return
	}
	logger.Debugf("removeNetwork request from %v: %v", req.RemoteAddr, spew.Sdump(p))

	if _, ok := r.session.Get(p.SessionID); ok == false {
		logger.Warningf("unknown session id: %v", p.SessionID)
		w.WriteJson(&api.Response{Status: api.StatusUnknownSession, Message: fmt.Sprintf("unknown session id: %v", p.SessionID)})
		return
	}

	if err := r.DB.RemoveNetwork(p.ID); err != nil {
		logger.Errorf("failed to remove network info: %v", err)
		w.WriteJson(&api.Response{Status: api.StatusInternalServerError, Message: err.Error()})
		return
	}
	logger.Debugf("removed network info: %v", spew.Sdump(p))

	logger.Debug("removing all flows from the entire switches")
	if err := r.Controller.RemoveFlows(); err != nil {
		// Ignore this error.
		logger.Errorf("failed to remove flows: %v", err)
	}
	logger.Debug("removed all flows from the entire switches")

	w.WriteJson(&api.Response{Status: api.StatusOkay})
}

type removeNetworkParam struct {
	SessionID string
	ID        uint64
}

func (r *removeNetworkParam) UnmarshalJSON(data []byte) error {
	v := struct {
		SessionID string `json:"session_id"`
		ID        uint64 `json:"id"`
	}{}
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*r = removeNetworkParam(v)

	return r.validate()
}

func (r *removeNetworkParam) validate() error {
	if len(r.SessionID) != 64 {
		return errors.New("invalid session id")
	}
	if r.ID == 0 {
		return errors.New("empty network id")
	}

	return nil
}
