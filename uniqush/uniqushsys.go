/*
 *  Uniqush by Nan Deng
 *  Copyright (C) 2010 Nan Deng
 *
 *  This software is free software; you can redistribute it and/or
 *  modify it under the terms of the GNU Lesser General Public
 *  License as published by the Free Software Foundation; either
 *  version 3.0 of the License, or (at your option) any later version.
 *
 *  This software is distributed in the hope that it will be useful,
 *  but WITHOUT ANY WARRANTY; without even the implied warranty of
 *  MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the GNU
 *  Lesser General Public License for more details.
 *
 *  You should have received a copy of the GNU Lesser General Public
 *  License along with this software; if not, write to the Free Software
 *  Foundation, Inc., 59 Temple Place, Suite 330, Boston, MA  02111-1307  USA
 *
 *  Nan Deng <monnand@gmail.com>
 *
 */

package uniqush

import (
    "goconf.googlecode.com/hg"
    "os"
    "io"
    "strings"
)

func loadLogInfo(c *conf.ConfigFile, field string, prefix string) (*Logger, os.Error) {
    var filename string
    var loglevel string
    var logswitch bool
    var err os.Error
    var writer io.Writer

    logswitch, err = c.GetBool(field, "log")
    if err != nil { logswitch = true }

    filename, err = c.GetString(field, "logfile")
    if err != nil {
        writer = os.Stderr
    } else {
        if filename == "" || filename == "STDERR" {
            writer = os.Stderr
        } else {
            writer, err = os.Create(filename)
            if err != nil {
                writer = os.Stderr
            }
        }
    }

    loglevel, err = c.GetString(field, "loglevel")
    if err != nil {
        loglevel = "standard"
    }
    var level int

    if logswitch {
        switch(strings.ToLower(loglevel)) {
        case "standard":
            level = LOGLEVEL_INFO
        case "verbose":
            level = LOGLEVEL_DEBUG
        default:
            level = LOGLEVEL_INFO
        }
    } else {
        level = LOGLEVEL_FATAL
    }

    logger := NewLogger(writer, prefix, level)
    return logger, nil
}

func loadDatabaseConfig(cf *conf.ConfigFile) (*DatabaseConfig, os.Error) {
    var err os.Error
    c := new(DatabaseConfig)
    c.Engine, err = cf.GetString("Database", "engine")
    if err != nil || c.Engine == "" {
        c.Engine = "redis"
    }
    c.Name, err = cf.GetString("Database", "name")
    if err != nil || c.Name == "" {
        c.Name = "0"
    }
    c.Port, err = cf.GetInt("Database", "port")
    if err != nil || c.Port <= 0 {
        c.Port = -1
    }
    c.Host, err = cf.GetString("Database", "host")
    if err != nil || c.Host == "" {
        c.Host = "localhost"
    }
    c.Password, err = cf.GetString("Database", "password")
    if err != nil {
        c.Password = ""
    }
    i, e := cf.GetInt("Database", "everysec")
    c.EverySec = int64(i)
    if e != nil || c.EverySec <= 60 {
        c.EverySec = 600
    }
    c.LeastDirty, err = cf.GetInt("Database", "leastdirty")
    if err != nil || c.LeastDirty < 0 {
        c.LeastDirty = 10
    }
    c.CacheSize, err = cf.GetInt("Database", "cachesize")
    if err != nil || c.CacheSize < 0 {
        c.CacheSize = 1024
    }
    return c, nil
}

type UniqushSystem struct {
    Frontend UniqushFrontEnd
    Backend UniqushBackEndIf
    Stopch chan bool
    Bridge chan *Request
    Database DatabaseFrontDeskIf
}

var (
    defaultConfigFilePath string = "/etc/uniqush/uniqush.conf"
)

func LoadUniqushSystem(filename string) (*UniqushSystem, os.Error) {
    if filename == "" {
        filename = defaultConfigFilePath
    }
    c, err := conf.ReadConfigFile(filename)
    if err != nil {
        return nil, err
    }

    ret := new(UniqushSystem)
    ret.Stopch = make(chan bool)
    ret.Bridge = make(chan *Request)
    ew := NewEventWriter(&NullWriter{})

    logger, e10 := loadLogInfo(c, "WebFrontend", "[WebFrontend]")
    if e10 != nil {
        return nil, e10
    }
    addr, e20 := c.GetString("WebFrontend", "addr")
    if e20 != nil || addr == "" {
        addr = "localhost:9898"
    }
    ret.Frontend = NewWebFrontEnd(ret.Bridge, logger, addr)
    ret.Frontend.SetStopChannel(ret.Stopch)
    ret.Frontend.SetEventWriter(ew)

    logger, e10 = loadLogInfo(c, "Backend", "[Backend]")
    if e10 != nil {
        return nil, e10
    }
    ret.Backend = NewUniqushBackEnd(ret.Bridge, logger)

    logger = NewLogger(os.Stdout, "[ActionPrinter]", LOGLEVEL_DEBUG)
    p := NewActionPrinter(logger)
    for i := 0; i < NR_ACTIONS; i++ {
        ret.Backend.SetProcessor(i, p)
    }

    /* Load Database */
    dbconf, e0 := loadDatabaseConfig(c)
    if e0 != nil {
        return nil, e0
    }
    dbf, e30 := NewDatabaseFrontDesk(dbconf)

    if e30 != nil{
        return nil, e30
    }
    ret.Database = dbf

    /* Load Processors */
    logger, e10 = loadLogInfo(c, "AddPushServiceProvider", "[AddPushServiceProvider]")
    if e10 != nil {
        return nil, e10
    }
    p = NewAddPushServiceProviderProcessor(logger, ew, dbf)
    ret.Backend.SetProcessor(ACTION_ADD_PUSH_SERVICE_PROVIDER, p)

    logger, e10 = loadLogInfo(c, "RemovePushServiceProvider", "[RemovePushServiceProvider]")
    if e10 != nil {
        return nil, e10
    }
    p = NewRemovePushServiceProviderProcessor(logger, ew, dbf)
    ret.Backend.SetProcessor(ACTION_REMOVE_PUSH_SERVICE_PROVIDER, p)

    logger, e10 = loadLogInfo(c, "Subscribe", "[Subscribe]")
    if e10 != nil {
        return nil, e10
    }
    p = NewSubscribeProcessor(logger, ew, dbf)
    ret.Backend.SetProcessor(ACTION_SUBSCRIBE, p)

    logger, e10 = loadLogInfo(c, "Unsubscribe", "[Unsubscribe]")
    if e10 != nil {
        return nil, e10
    }
    p = NewSubscribeProcessor(logger, ew, dbf)
    ret.Backend.SetProcessor(ACTION_SUBSCRIBE, p)

    logger, e10 = loadLogInfo(c, "Push", "[Push]")
    if e10 != nil {
        return nil, e10
    }
    p = NewPushProcessor(logger, ew, dbf, ret.Bridge)
    ret.Backend.SetProcessor(ACTION_PUSH, p)
    return ret, nil
}

func (s *UniqushSystem) Run() {
    go s.Frontend.Run()
    go s.Backend.Run()
    <-s.Stopch
    s.Database.FlushCache()
}

