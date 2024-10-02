/*
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package designate

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strconv"
	"text/template"
)

type Pool struct {
	Name        string
	Description string
	Attributes  map[string]string
	NSRecords   []NSRecord
	Nameservers []Nameserver
	Targets     []Target
	CatalogZone *CatalogZone // it is a pointer because it is optional
}

type NSRecord struct {
	Hostname string
	Priority int
}

type Nameserver struct {
	Host string
	Port int
}

type Target struct {
	Type        string
	Description string
	Masters     []Master
	Options     Options
}

type Master struct {
	Host string
	Port int
}

type Options struct {
	Host           string
	Port           int
	RNDCHost       string
	RNDCPort       int
	RNDCConfigFile string
}

type CatalogZone struct {
	FQDN    string
	Refresh string
}

const poolsTemplatesDir = "templates/designate/config"

func GeneratePoolsYamlData(BindMap, MdnsMap, NsRecordsMap map[string]string) (string, error) {
	// Create ns_records
	nsRecords := []NSRecord{}
	for hostname, priority := range NsRecordsMap {
		priority, err := strconv.Atoi(priority)
		if err != nil {
			return "", err
		}
		nsRecord := NSRecord{
			Hostname: hostname,
			Priority: priority,
		}
		nsRecords = append(nsRecords, nsRecord)
	}

	// Create targets and nameservers
	nameservers := []Nameserver{}
	targets := []Target{}
	rndcKeyNum := 1

	for _, bindIP := range BindMap {
		nameservers = append(nameservers, Nameserver{
			Host: bindIP,
			Port: 53,
		})

		masters := []Master{}
		for _, masterHost := range MdnsMap {
			masters = append(masters, Master{
				Host: masterHost,
				Port: 5354,
			})
		}

		target := Target{
			Type:        "bind9",
			Description: fmt.Sprintf("BIND9 Server %d (%s)", rndcKeyNum, bindIP),
			Masters:     masters,
			Options: Options{
				Host:           bindIP,
				Port:           53,
				RNDCHost:       bindIP,
				RNDCPort:       953,
				RNDCConfigFile: fmt.Sprintf("%s/%s-%d.conf", RndcConfDir, DesignateRndcKey, rndcKeyNum),
			},
		}
		targets = append(targets, target)
		rndcKeyNum++
	}

	// Catalog zone is an optional section
	// catalogZone := &CatalogZone{
	// 	FQDN:    "example.org.",
	// 	Refresh: "60",
	// }
	defaultAttributes := make(map[string]string)
	// Create the Pool struct with the dynamic values
	pool := Pool{
		Name:        "default",
		Description: "Default BIND Pool",
		Attributes:  defaultAttributes,
		NSRecords:   nsRecords,
		Nameservers: nameservers,
		Targets:     targets,
		CatalogZone: nil, // set to catalogZone if this section should be presented
	}

	// Parse the template and return it as a string
	templatePath := filepath.Join(poolsTemplatesDir, "pools.yaml.tmpl")
	tmpl, err := template.ParseFiles(templatePath)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, pool)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}
