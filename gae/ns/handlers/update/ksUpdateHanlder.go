// Copyright 2013 M-Lab
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// +build appengine

package update

import (
	"appengine"
	"appengine/datastore"
	"appengine/urlfetch"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"code.google.com/p/mlab-ns2/gae/ns/data"
)

const (
	KsIPUrl            = "http://ks.measurementlab.net/mlab-host-ips.txt"
	KsUpdateHandlerUrl = "/admin/KsUpdateHandler"
)

func init() {
	http.HandleFunc(KsUpdateHandlerUrl, KsUpdateHandler)
}

// KsUpdateHandler handles IP address updates
func KsUpdateHandler(w http.ResponseWriter, r *http.Request) {

	c := appengine.NewContext(r)
	client := urlfetch.Client(c)
	res, err := client.Get(KsIPUrl)
	if err != nil {
		c.Errorf("KsUpdateHandler:client.Get(KsIPUrl) err = %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	txtBlob, _ := ioutil.ReadAll(res.Body)
	defer res.Body.Close()
	txt := fmt.Sprintf("%s", txtBlob)
	lines := strings.Split(txt, "\n")
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		split := strings.Split(line, ",")
		fqdn := split[0]
		ipv4 := split[1]
		ipv6 := split[2]
		q := datastore.NewQuery("SliverTool").Filter("fqdn=", fqdn)
		var sliverTool []*data.SliverTool
		_, err = q.GetAll(c, &sliverTool)
		if err != nil {
			c.Errorf("KsUpdateHandler:q.GetAll(..sliverTool) err = %v", err)
			continue
		}
		if len(sliverTool) > 1 {
			c.Errorf("KsUpdateHandler:q.GetAll(..sliverTool): Two sliverTools with same fqdn = %s", fqdn)
			continue
		} else if len(sliverTool) < 1 {
			c.Errorf("KsUpdateHandler:q.GetAll(..sliverTool): sliverTool not found with fqdn = %s", fqdn)
			continue
		}

		sliverTool[0].SliverIPv4 = ipv4
		if ipv4 == "" {
			sliverTool[0].SliverIPv4 = "off"
		}
		sliverTool[0].SliverIPv6 = ipv6
		if ipv6 == "" {
			sliverTool[0].SliverIPv6 = "off"
		}

		slID := data.GetSliverToolID(sliverTool[0].ToolID, sliverTool[0].SliceID, sliverTool[0].ServerID, sliverTool[0].SiteID)
		sk := datastore.NewKey(c, "SliverTool", slID, 0, nil)
		_, err := datastore.Put(c, sk, sliverTool[0])
		if err != nil {
			c.Errorf("KsUpdateHandler: datastore.Put err %v ", err)
		} else {
			// TODO: add to data to update LocationMap and memcache
		}
	}
	// TODO: Update locationMap
	// geo.LMapIPv4.ChangeMulti(map[oldIP]newIP)
	// geo.LMapIPv6.ChangeMulti(map[oldIP]newIP)
	// TODO: Update memcache
	fmt.Fprintf(w, "OK")
}
