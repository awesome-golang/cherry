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

package database

import (
	"database/sql"
	"errors"
	"fmt"
	"github.com/dlintw/goconf"
	"github.com/go-sql-driver/mysql"
	"net"
)

const (
	deadlockErrCode  uint16 = 1213
	maxDeadlockRetry        = 5
)

type MySQL struct {
	db *sql.DB
}

func NewMySQL(conf *goconf.ConfigFile) (*MySQL, error) {
	db, err := newDBConn(conf)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(32)
	db.SetMaxIdleConns(4)

	mysql := &MySQL{
		db: db,
	}
	if err := mysql.createTables(); err != nil {
		return nil, err
	}

	return mysql, nil
}

func newDBConn(conf *goconf.ConfigFile) (*sql.DB, error) {
	host, err := conf.GetString("database", "host")
	if err != nil || len(host) == 0 {
		return nil, errors.New("empty database host in the config file")
	}
	port, err := conf.GetInt("database", "port")
	if err != nil || port <= 0 || port > 0xFFFF {
		return nil, errors.New("invalid database port in the config file")
	}
	user, err := conf.GetString("database", "user")
	if err != nil || len(user) == 0 {
		return nil, errors.New("empty database user in the config file")
	}
	password, err := conf.GetString("database", "password")
	if err != nil || len(password) == 0 {
		return nil, errors.New("empty database password in the config file")
	}
	dbname, err := conf.GetString("database", "name")
	if err != nil || len(dbname) == 0 {
		return nil, errors.New("empty database name in the config file")
	}

	db, err := sql.Open("mysql", fmt.Sprintf("%v:%v@tcp(%v:%v)/%v?timeout=5s", user, password, host, port, dbname))
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}

	return db, nil
}

func isDeadlock(err error) bool {
	e, ok := err.(*mysql.MySQLError)
	if !ok {
		return false
	}

	return e.Number == deadlockErrCode
}

func query(f func() error) error {
	deadlockRetry := 0
	for {
		err := f()
		if err == nil {
			return nil
		}
		if !isDeadlock(err) || deadlockRetry >= maxDeadlockRetry {
			return err
		}
		deadlockRetry++
	}
}

func (r *MySQL) MAC(ip net.IP) (mac net.HardwareAddr, ok bool, err error) {
	if ip == nil {
		panic("IP address is nil")
	}

	f := func() error {
		qry := `SELECT mac 
			FROM host A 
			JOIN ip B 
			ON A.ip_id = B.id 
			WHERE B.address = INET_ATON(?)`
		row, err := r.db.Query(qry, ip.String())
		if err != nil {
			return err
		}
		defer row.Close()

		// Unknown IP address?
		if !row.Next() {
			return nil
		}
		if err := row.Err(); err != nil {
			return err
		}

		var v []byte
		if err := row.Scan(&v); err != nil {
			return err
		}
		if v == nil || len(v) != 6 {
			panic("Invalid MAC address")
		}
		mac = net.HardwareAddr(v)
		ok = true

		return nil
	}
	err = query(f)

	return mac, ok, err
}

func (r *MySQL) Location(mac net.HardwareAddr) (dpid string, port uint32, ok bool, err error) {
	if mac == nil {
		panic("MAC address is nil")
	}

	f := func() error {
		qry := `SELECT A.dpid, B.number 
			FROM switch A 
			JOIN port B 
			ON B.switch_id = A.id 
			JOIN host C 
			ON C.port_id = B.id 
			WHERE C.mac = ?
			GROUP BY(A.dpid)`
		row, err := r.db.Query(qry, []byte(mac))
		if err != nil {
			return err
		}
		defer row.Close()

		// Unknown MAC address?
		if !row.Next() {
			return nil
		}
		if err := row.Err(); err != nil {
			return err
		}

		if err := row.Scan(&dpid, &port); err != nil {
			return err
		}
		ok = true

		return nil
	}
	err = query(f)

	return dpid, port, ok, err
}