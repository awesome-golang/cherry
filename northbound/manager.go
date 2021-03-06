/*
 * Cherry - An OpenFlow Controller
 *
 * Copyright (C) 2015 Samjung Data Service, Inc. All rights reserved.
 * Kitae Kim <superkkt@sds.co.kr>
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

package northbound

import (
	"bytes"
	"fmt"
	"strings"
	"sync"

	"github.com/superkkt/cherry/database"
	"github.com/superkkt/cherry/network"
	"github.com/superkkt/cherry/northbound/app"
	"github.com/superkkt/cherry/northbound/app/announcer"
	"github.com/superkkt/cherry/northbound/app/discovery"
	"github.com/superkkt/cherry/northbound/app/l2switch"
	"github.com/superkkt/cherry/northbound/app/monitor"
	"github.com/superkkt/cherry/northbound/app/proxyarp"
	"github.com/superkkt/cherry/northbound/app/virtualip"

	"github.com/pkg/errors"
	"github.com/superkkt/go-logging"
)

var (
	logger = logging.MustGetLogger("northbound")
)

type EventSender interface {
	SetEventListener(network.EventListener)
}

type application struct {
	instance app.Processor
	enabled  bool
}

type Manager struct {
	mutex      sync.Mutex
	apps       map[string]*application // Registered applications
	head, tail app.Processor
	db         *database.MySQL
}

func NewManager(db *database.MySQL) (*Manager, error) {
	v := &Manager{
		apps: make(map[string]*application),
		db:   db,
	}
	// Registering north-bound applications
	v.register(discovery.New(db))
	v.register(l2switch.New(db))
	v.register(proxyarp.New(db))
	v.register(monitor.New())
	v.register(virtualip.New(db))
	v.register(announcer.New(db))

	return v, nil
}

func (r *Manager) register(app app.Processor) {
	r.apps[strings.ToUpper(app.Name())] = &application{
		instance: app,
		enabled:  false,
	}
}

// XXX: Caller should lock the mutex before they call this function
func (r *Manager) checkDependencies(appNames []string) error {
	if appNames == nil || len(appNames) == 0 {
		// No dependency
		return nil
	}

	for _, name := range appNames {
		app, ok := r.apps[strings.ToUpper(name)]
		logger.Debugf("app: %+v, ok: %v", app, ok)
		if !ok || !app.enabled {
			return fmt.Errorf("%v application is not loaded", name)
		}
	}

	return nil
}

func (r *Manager) Enable(appName string) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	logger.Debugf("enabling %v application..", appName)
	v, ok := r.apps[strings.ToUpper(appName)]
	if !ok {
		return fmt.Errorf("unknown application: %v", appName)
	}
	if v.enabled == true {
		logger.Debugf("%v: already enabled", appName)
		return nil
	}

	app := v.instance
	if err := app.Init(); err != nil {
		return errors.Wrap(err, "initializing application")
	}
	if err := r.checkDependencies(app.Dependencies()); err != nil {
		return errors.Wrap(err, "checking dependencies")
	}
	v.enabled = true
	logger.Debugf("enabled %v application", appName)

	if r.head == nil {
		r.head = app
		r.tail = app
		return nil
	}
	r.tail.SetNext(app)
	r.tail = app

	return nil
}

func (r *Manager) AddEventSender(sender EventSender) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if r.head == nil {
		return
	}
	sender.SetEventListener(r.head)
}

func (r *Manager) String() string {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	var buf bytes.Buffer
	app := r.head
	for app != nil {
		buf.WriteString(fmt.Sprintf("%v\n", app))
		next, ok := app.Next()
		if !ok {
			break
		}
		app = next
	}

	return buf.String()
}
