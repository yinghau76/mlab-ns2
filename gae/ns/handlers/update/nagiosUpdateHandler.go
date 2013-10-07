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
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"code.google.com/p/mlab-ns2/gae/ns/data"
	"code.google.com/p/mlab-ns2/gae/ns/digest"
)

const (
	DefaultNagiosEntry     = "0"
	StatusOnline           = "online"
	StatusOffline          = "offline"
	NagiosServiceStatusOk  = "0"
	NagiosUpdateHandlerURL = "/admin/NagiosUpdateHandler"
)

var (
	familys = [...]string{"", "_ipv6"}
)

func init() {
	http.HandleFunc(NagiosUpdateHandlerURL, NagiosUpdateHandler)
}

func process(line string) (string, string) {
	split := strings.Split(line, " ")
	if len(split) < 2 {
		return "", ""
	}
	sliceFQDN := split[0]
	state := split[1]
	sliverFQDN := strings.Split(sliceFQDN, "/")
	if len(sliverFQDN) < 1 {
		return "", ""
	}
	return state, sliverFQDN[0]
}

func getSliceStatus(t *digest.Transport, url string) (map[string]string, error) {

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	res, err := t.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	txtBlob, _ := ioutil.ReadAll(res.Body)
	txt := fmt.Sprintf("%s", txtBlob)
	defer res.Body.Close()

	sliceStatus := make(map[string]string)
	lines := strings.Split(txt, "\n")
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		state, sliverFQDN := process(line)
		if state == "" {
			continue
		}
		sliceStatus[sliverFQDN] = StatusOnline
		if state != NagiosServiceStatusOk {
			sliceStatus[sliverFQDN] = StatusOffline
		}
	}
	if sliceStatus == nil {
		return nil, errors.New("sliceStatus is nil")
	}
	return sliceStatus, nil
}

func updateSliverToolStatus(c appengine.Context, sliceStatus map[string]string, toolID, family string) error {

	q := datastore.NewQuery("SliverTool").Filter("tool_id =", toolID)
	var sliverTools []*data.SliverTool
	_, err := q.GetAll(c, &sliverTools)
	if err != nil {
		return err
	}
	count, err := q.Count(c)
	if err != nil {
		return err
	}
	updatedSliverTools := make([]*data.SliverTool, count)
	for _, sl := range sliverTools {
		isMatch := false
		for sliverFQDN, status := range sliceStatus {
			if sl.FQDN == sliverFQDN {
				if family == " " {
					sl.StatusIPv4 = status
					if sl.SliverIPv4 == "off" {
						sl.StatusIPv4 = StatusOffline
					}
				} else {
					sl.StatusIPv6 = status
					if sl.SliverIPv6 == "off" {
						sl.StatusIPv6 = StatusOffline
					}
				}
				isMatch = true
				break
			}
		}
		if isMatch {
			sl.When = time.Now()
			slID := data.GetSliverToolID(sl.ToolID, sl.SliceID, sl.ServerID, sl.SiteID)
			sk := datastore.NewKey(c, "SliverTool", slID, 0, nil)
			_, err := datastore.Put(c, sk, sl)
			if err != nil {
				c.Errorf("updateSliverToolStatus: datastore.Put err %v", err)
			} else {
				updatedSliverTools = append(updatedSliverTools, sl)
			}
		}
	}
	//TODO: Update locationMap
	// convert updatedSliverTools and sliverTools to list locmap.Data
	if family == " " {
		// geo.LMapIPv4.UpdateMulti(updatedSliverTools, sliverTools)
	} else {
		// geo.LMapIPv6.UpdateMulti(updatedSliverTools, sliverTools)
	}
	//TODO: Update memcache
	return nil
}

// NagiosUpdateHandler updates the status of SliverTools
func NagiosUpdateHandler(w http.ResponseWriter, r *http.Request) {

	c := appengine.NewContext(r)

	// Get Nagios Data
	q := datastore.NewQuery("Nagios").Filter("key_id=", DefaultNagiosEntry)
	var nagiosd []*data.Nagios
	_, err := q.GetAll(c, &nagiosd)
	if err != nil {
		c.Errorf("NagiosUpdateHandler:q.GetAll(..nagios) err = %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	nagios := *nagiosd[0]

	// Get Slices
	q = datastore.NewQuery("Slice")
	var slices []*data.Slice
	_, err = q.GetAll(c, &slices)
	if err != nil {
		c.Errorf("NagiosUpdateHandler:q.GetAll(..slices) err = %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Update SliverTools
	t := digest.GAETransport(c, nagios.Username, nagios.Password)
	for _, slice := range slices {
		for _, family := range familys {
			url := nagios.URL + "?show_state=1&service_name=" + slice.ToolID + family
			sliceStatus, err := getSliceStatus(t, url)
			if err != nil {
				c.Errorf("NagiosUpdateHandler:getSliceStatus(%s) err = %v", slice.ToolID, err)
				continue
			}
			err = updateSliverToolStatus(c, sliceStatus, slice.ToolID, family)
			if err != nil {
				c.Errorf("NagiosUpdateHandler:updateSliverToolStatus(%s,%s) err = %v", slice.ToolID, family, err)
			}
		}
	}
	fmt.Fprintf(w, "OK")
}
