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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path"
	"sort"
	"text/template"

	designate "github.com/openstack-k8s-operators/designate-operator/api/v1beta1"
	"github.com/openstack-k8s-operators/lib-common/modules/common/util"
	"gopkg.in/yaml.v2"
)

type Pool struct {
	Name        string            `yaml:"name"`
	Description string            `yaml:"description"`
	Attributes  map[string]string `yaml:"attributes"`
	NSRecords   []NSRecord        `yaml:"ns_records"`
	Nameservers []Nameserver      `yaml:"nameservers"`
	Targets     []Target          `yaml:"targets"`
	CatalogZone *CatalogZone      `yaml:"catalog_zone,omitempty"` // it is a pointer because it is optional
}

type NSRecord struct {
	Hostname string `yaml:"hostname"`
	Priority int    `yaml:"priority"`
}

type Nameserver struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type Target struct {
	Type        string   `yaml:"type"`
	Description string   `yaml:"description"`
	Masters     []Master `yaml:"masters"`
	Options     Options  `yaml:"options"`
}

type Master struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type Options struct {
	Host        string `yaml:"host"`
	Port        int    `yaml:"port"`
	RNDCHost    string `yaml:"rndc_host"`
	RNDCPort    int    `yaml:"rndc_port"`
	RNDCKeyFile string `yaml:"rndc_key_file"`
}

type CatalogZone struct {
	FQDN    string `yaml:"fqdn"`
	Refresh int    `yaml:"refresh"`
}

// We sort all pool resources to get the correct hash every time
func GeneratePoolsYamlDataAndHash(BindMap, MdnsMap map[string]string, designateNSRecords []designate.DesignateNSRecord) (string, string, error) {
	nsRecords := make([]NSRecord, len(designateNSRecords))
	for i, record := range designateNSRecords {
		nsRecords[i] = NSRecord{
			Hostname: record.Hostname,
			Priority: record.Priority,
		}
	}
	sort.Slice(nsRecords, func(i, j int) bool {
		if nsRecords[i].Hostname != nsRecords[j].Hostname {
			return nsRecords[i].Hostname < nsRecords[j].Hostname
		}
		return nsRecords[i].Priority < nsRecords[j].Priority
	})

	bindIPs := make([]string, 0, len(BindMap))
	for _, ip := range BindMap {
		bindIPs = append(bindIPs, ip)
	}
	sort.Strings(bindIPs)

	masterHosts := make([]string, 0, len(MdnsMap))
	for _, host := range MdnsMap {
		masterHosts = append(masterHosts, host)
	}
	sort.Strings(masterHosts)

	nameservers := make([]Nameserver, len(bindIPs))
	for i, bindIP := range bindIPs {
		nameservers[i] = Nameserver{
			Host: bindIP,
			Port: 53,
		}
	}

	targets := make([]Target, len(bindIPs))
	for i, bindIP := range bindIPs {
		masters := make([]Master, len(masterHosts))
		for j, masterHost := range masterHosts {
			masters[j] = Master{
				Host: masterHost,
				Port: 5354,
			}
		}

		targets[i] = Target{
			Type:        "bind9",
			Description: fmt.Sprintf("BIND9 Server %d (%s)", i, bindIP),
			Masters:     masters,
			Options: Options{
				Host:        bindIP,
				Port:        53,
				RNDCHost:    bindIP,
				RNDCPort:    953,
				RNDCKeyFile: fmt.Sprintf("%s/%s-%d", RndcConfDir, DesignateRndcKey, i),
			},
		}
	}

	sort.Slice(targets, func(i, j int) bool {
		return targets[i].Options.Host < targets[j].Options.Host
	})

	for i := range targets {
		targets[i].Description = fmt.Sprintf("BIND9 Server %d (%s)", i, targets[i].Options.Host)
		targets[i].Options.RNDCKeyFile = fmt.Sprintf("%s/%s-%d", RndcConfDir, DesignateRndcKey, i)
	}

	// Catalog zone is an optional section
	// catalogZone := &CatalogZone{
	// 	FQDN:    "example.org.",
	//	Refresh: 60,
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

	poolBytes, err := yaml.Marshal(pool)
	if err != nil {
		return "", "", fmt.Errorf("error marshalling pool for hash: %w", err)
	}
	hasher := sha256.New()
	hasher.Write(poolBytes)
	poolHash := hex.EncodeToString(hasher.Sum(nil))

	opTemplates, err := util.GetTemplatesPath()
	if err != nil {
		return "", "", err
	}
	poolsYamlPath := path.Join(opTemplates, PoolsYamlPath)
	poolsYaml, err := os.ReadFile(poolsYamlPath)
	if err != nil {
		return "", "", err
	}
	tmpl, err := template.New("pool").Parse(string(poolsYaml))
	if err != nil {
		return "", "", err
	}
	var buf bytes.Buffer
	err = tmpl.Execute(&buf, pool)
	if err != nil {
		return "", "", err
	}

	return buf.String(), poolHash, nil
}
