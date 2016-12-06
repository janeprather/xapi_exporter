package main

import (
	"fmt"
	"log"
	"strings"
	"time"

	xenAPI "github.com/janeprather/go-xen-api-client"
)

func getXenClient(pool string) (
	xenClient *xenAPI.Client, session xenAPI.SessionRef, err error) {

	for _, host := range config.Pools[pool] {
		xenClient, session, err = tryXenClient(host)
		if err == nil {
			return xenClient, session, nil
		}
		log.Printf("tryXenClient(): %s: %s\n", host, err.Error())
	}

	return nil, "", fmt.Errorf(
		"%s: unable to authenticate into a master host", pool)
}

func tryXenClient(host string) (
	xenClient *xenAPI.Client, session xenAPI.SessionRef, err error) {

	xenClient, err = xenAPI.NewClient("https://"+host, nil)
	if err != nil {
		return nil, "", fmt.Errorf("NewClient(): %s: %s\n", host, err.Error())
	}

	sessionCh := make(chan xenAPI.SessionRef)
	errCh := make(chan error)
	go func() {
		session, err = xenClient.Session.LoginWithPassword(
			config.Auth.Username, config.Auth.Password,
			"1.0", "xapi_exporter")
		if err != nil {
			errCh <- err
		} else {
			sessionCh <- session
		}
	}()

	select {
	case err := <-errCh:
		errParts := strings.Split(err.Error(), " ")
		if errParts[2] == "HOST_IS_SLAVE" {
			return tryXenClient(errParts[3])
		}
		return nil, "", fmt.Errorf(
			"LoginWithPassword(): %s: %s\n", host, err.Error())
	case <-time.After(time.Second * config.TimeoutLogin):
		return nil, "", fmt.Errorf(
			"LoginWithPassword(): timeout after %d seconds", config.TimeoutLogin)
	case session = <-sessionCh:
	}

	return
}
