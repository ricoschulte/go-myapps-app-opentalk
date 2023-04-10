# go-myapps-app-opentalk

innovaphone myApps Presence Integration for Opentalk 

> **Warning**
> This project currently works only with a customized version of the Opentalk controller! Please see https://github.com/ricoschulte/opentalk-controller/pull/2 for details.

## Install

Add this to your docker-compose stack where Opentalk is running.

```YAML
---
version: "3.9"
services:
  # *** go-myapps-app-opentalk ***
  go-myapps-app-opentalk:
    image: rschulte/go-myapps-app-opentalk:latest
    restart: always
    ports:
      # http port
      - "3000:3000"
      # http/tls port
      - "3001:3001"
    command: |
      -domain example.com
      -name opentalk
      -instance demo
      -password your-secret-password
      -redis_host ${REDIS_HOST:-redis}
      -pg_host ${POSTGRES_HOST:-postgres}
      -pg_port ${POSTGRES_PORT:-5432}
      -pg_user ${POSTGRES_USER:-ot}
      -pg_password ${POSTGRES_PASSWORD}
      -pg_database ${POSTGRES_DB:-k3k}
# Optional     -loglevel debug

## rest of the opentalk compose stack https://gitlab.opencode.de/opentalk/ot-setup/-/blob/main/lite/docker-compose.yaml
```

## About ©

[myApps](https://www.innovaphone.com/en/myapps/what-is-myapps.html) is a product of [innovaphone AG](https://www.innovaphone.com). Documentation of the API used in this client can be found at [ innovaphone App SDK](https://sdk.innovaphone.com/).

[Opentalk](https://opentalk.eu) is a product of [OpenTalk GmbH](https://opentalk.eu/de/impressum)