// Package v061 package implements support for interacting with
// Alertmanager 0.6.1
// Collected data will be mapped to unsee internal schema defined the
// unsee/models package
// This file defines Alertmanager alerts mapping
package v061

import (
	"errors"
	"time"

	"github.com/blang/semver"
	"github.com/cloudflare/unsee/config"
	"github.com/cloudflare/unsee/mapper"
	"github.com/cloudflare/unsee/models"
	"github.com/cloudflare/unsee/transport"
)

type alert struct {
	Annotations  map[string]string `json:"annotations"`
	Labels       map[string]string `json:"labels"`
	StartsAt     time.Time         `json:"startsAt"`
	EndsAt       time.Time         `json:"endsAt"`
	GeneratorURL string            `json:"generatorURL"`
	Status       string            `json:"Status"`
	SilencedBy   []string          `json:"silencedBy"`
	InhibitedBy  []string          `json:"inhibitedBy"`
}

type alertsGroups struct {
	Labels map[string]string `json:"labels"`
	Blocks []struct {
		Alerts   []alert `json:"alerts"`
		RouteOps struct {
			Receiver string `json:"receiver"`
		} `json:"routeOpts"`
	} `json:"blocks"`
}

type alertsGroupsAPISchema struct {
	Status string         `json:"status"`
	Data   []alertsGroups `json:"data"`
	Error  string         `json:"error"`
}

type alertsGroupReceiver struct {
	Name   string
	Groups []models.AlertGroup
}

// AlertMapper implements Alertmanager API schema
type AlertMapper struct {
	mapper.AlertMapper
}

// IsSupported returns true if given version string is supported
func (m AlertMapper) IsSupported(version string) bool {
	versionRange := semver.MustParseRange("=0.6.1")
	return versionRange(semver.MustParse(version))
}

// GetAlerts will make a request to Alertmanager API and parse the response
// It will only return alerts or error (if any)
func (m AlertMapper) GetAlerts() ([]models.AlertGroup, error) {
	groups := []models.AlertGroup{}
	receivers := map[string]alertsGroupReceiver{}
	resp := alertsGroupsAPISchema{}

	url, err := transport.JoinURL(config.Config.AlertmanagerURI, "api/v1/alerts/groups")
	if err != nil {
		return groups, err
	}

	err = transport.ReadJSON(url, config.Config.AlertmanagerTimeout, &resp)
	if err != nil {
		return groups, err
	}

	if resp.Status != "success" {
		return groups, errors.New(resp.Error)
	}

	for _, d := range resp.Data {
		for _, b := range d.Blocks {
			rcv, found := receivers[b.RouteOps.Receiver]
			if !found {
				rcv = alertsGroupReceiver{
					Name: b.RouteOps.Receiver,
				}
				receivers[b.RouteOps.Receiver] = rcv
			}
			alertList := models.AlertList{}
			for _, a := range b.Alerts {
				inhibitedBy := []string{}
				if a.InhibitedBy != nil {
					inhibitedBy = a.InhibitedBy
				}
				silencedBy := []string{}
				if a.SilencedBy != nil {
					silencedBy = a.SilencedBy
				}
				a := models.Alert{
					Receiver:     rcv.Name,
					Annotations:  a.Annotations,
					Labels:       a.Labels,
					StartsAt:     a.StartsAt,
					EndsAt:       a.EndsAt,
					GeneratorURL: a.GeneratorURL,
					State:        a.Status,
					InhibitedBy:  inhibitedBy,
					SilencedBy:   silencedBy,
				}
				alertList = append(alertList, a)
			}
			ug := models.AlertGroup{
				Receiver: rcv.Name,
				Labels:   d.Labels,
				Alerts:   alertList,
			}
			rcv.Groups = append(rcv.Groups, ug)
			receivers[rcv.Name] = rcv
		}
	}
	for _, rcv := range receivers {
		for _, ag := range rcv.Groups {
			groups = append(groups, ag)
		}
	}
	return groups, nil
}
