package main

import (
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/ricoschulte/go-myapps/service"
	"github.com/ricoschulte/go-myapps/service/pbxapi"
	"github.com/ricoschulte/go-myapps/service/pbxtableusers"
	log "github.com/sirupsen/logrus"
)

var version string // version is set at compile time using -ldflags

//go:embed static/*
var StaticFiles embed.FS

var staticDir = flag.String("staticdir", "", "path to a localdir served via http at /domain/name/instance/static/")
var log_level = flag.String("loglevel", "info", "log level to use")
var domain = flag.String("domain", "", "domain part of the listening websocket url '/domain/name/instance'")
var name = flag.String("name", "", "name part of the listening websocket url '/domain/name/instance'")
var instance = flag.String("instance", "", "instance part of the listening websocket url '/domain/name/instance'")
var password = flag.String("password", "", "password for the app")
var port_http = flag.Int("port_http", 3000, "http port listening for websockets from the pbx/myapps")
var port_https = flag.Int("port_https", 3001, "http/tls port listening for websockets from the pbx/myapps")
var tlsCertKeyFile = flag.String("certkeyfile", "", "path to a file with the private key of the tls certificate. if not set a default certificate is used")
var tlsCertFile = flag.String("certfile", "", "path to a file with the certificate of the tls certificate. if not set a default certificate is used")

// settings for redis
var redis_host = flag.String("redis_host", "redis", "redis_host")
var redis_port = flag.Int("redis_port", 6379, "redis_port")
var redis_username = flag.String("redis_username", "", "redis_username")
var redis_password = flag.String("redis_password", "", "redis_password")
var redis_db = flag.Int("redis_db", 0, "redis_db")
var redis_listen_topic = "k3k-signaling:room=*:participant=*:participant:event"

// settings for postgres
var pg_host = flag.String("pg_host", "postgres", "pg_host")
var pg_port = flag.Int("pg_port", 5432, "pg_port")
var pg_user = flag.String("pg_user", "", "pg_user")
var pg_password = flag.String("pg_password", "", "pg_password")
var pg_database = flag.String("pg_database", "", "pg_database")

// text to set to the user busy presence
var pbx_presence_note = "Free Version / Opentalk Meeting"

// default ssl cert for incoming http/websocket connections
var certificate = `-----BEGIN CERTIFICATE-----
MIIDazCCAlOgAwIBAgIUXSAwasqE4yA4YvbGqG9L4CIlFrEwDQYJKoZIhvcNAQEL
BQAwRTELMAkGA1UEBhMCQVUxEzARBgNVBAgMClNvbWUtU3RhdGUxITAfBgNVBAoM
GEludGVybmV0IFdpZGdpdHMgUHR5IEx0ZDAeFw0yMzAzMDYwNTMxMjRaFw0yNDAz
MDUwNTMxMjRaMEUxCzAJBgNVBAYTAkFVMRMwEQYDVQQIDApTb21lLVN0YXRlMSEw
HwYDVQQKDBhJbnRlcm5ldCBXaWRnaXRzIFB0eSBMdGQwggEiMA0GCSqGSIb3DQEB
AQUAA4IBDwAwggEKAoIBAQCy1Jml6FeX4RLfOwurSlyhiDSQeYbtgHHRqyxEoxwC
D+zeL5MCzOH7QJY4f3byi0DLJY+zc021VylgK+m1v2DCAcOsp6z7NSXD9jhrJEWV
MZBfDv8uKXDfwhCoQ4qUd86IduuaX6I4pHQ/+oAg06sX3TPyugGNb1iv/mbcJaxN
kzKDDyC06p9ICTR++zTRq2MhNjmWda791LNOGxSg0o0KJ1oDjI554wqWXsHVcHwF
URZGwapA2Q9yD735EpeKv5BZejIpSVP38Ak8qiofmqi61VSVIGtH8Vrp7M3it3cE
WuQrYmuIwlmXD33BZipQ4m8KdA7JOkiIUJsMalY+mAvVAgMBAAGjUzBRMB0GA1Ud
DgQWBBSkOtCe5FxQl5xCBgluYKEO/s/27TAfBgNVHSMEGDAWgBSkOtCe5FxQl5xC
BgluYKEO/s/27TAPBgNVHRMBAf8EBTADAQH/MA0GCSqGSIb3DQEBCwUAA4IBAQAT
QtHqOEWQQfO4byn9PQSPmfSdEeOR7SwJKic/nAptJbNT2FoyLzSiomaNzbSD3Q/5
8HawjkH8PABthoJ5iD2txxzjnAeSb2UgYGrYfVFWHKNInlTi/RkMo0n91fNm/TC4
IimAEG8uWDyz8hn9P9WYR4Qkbtn+rsKzHB6ygToXmwAY3zZ0hPPzRgLTICPHZa1D
epEcCnO7ftVqRa1vI4S6svXM2WxBV7ziJ5WD0jZSYH6yfzqsxh2TEO8v3q8LDUzM
stTz602vnsyQ91ELeu2yLuwICelYe5gZH/nsOYtJinIZjzoI8tJEKIG36rb1Qo+d
5DBk2bIg2wqJnQ5Ns08C
-----END CERTIFICATE-----`

// default ssl certkey for incoming http/websocket connections
var certificateKey = `-----BEGIN PRIVATE KEY-----
MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQCy1Jml6FeX4RLf
OwurSlyhiDSQeYbtgHHRqyxEoxwCD+zeL5MCzOH7QJY4f3byi0DLJY+zc021Vylg
K+m1v2DCAcOsp6z7NSXD9jhrJEWVMZBfDv8uKXDfwhCoQ4qUd86IduuaX6I4pHQ/
+oAg06sX3TPyugGNb1iv/mbcJaxNkzKDDyC06p9ICTR++zTRq2MhNjmWda791LNO
GxSg0o0KJ1oDjI554wqWXsHVcHwFURZGwapA2Q9yD735EpeKv5BZejIpSVP38Ak8
qiofmqi61VSVIGtH8Vrp7M3it3cEWuQrYmuIwlmXD33BZipQ4m8KdA7JOkiIUJsM
alY+mAvVAgMBAAECggEAeymQ6IKsUR3iMXwo/T+prFZyXU5Vbx0XRp/tTRhJIeJ1
8FAzn6obuT8yNpcTBNiDN2YXIjA3RL1S8blMrK+xo+wzJ6YTrK9d4yigkqnYgngw
Rke9170S0AiIEFr0Bmy9AZ9lhFx5DSm2JpoPxIwDOdxO+szAZPhazFsZ3GTV1lZz
qNF0nX52cSB/ok9L6DTn9O6KVIMkoAZH3vUg1rHvxe0pA144/zoWhYBG064W8QfZ
cNlqfMrgc+/eZGNeUSMCoQqRTEYmFRetacl8fmUuJD7FTBECZuUPr8ywUFLR1/nG
ojCGUfDnmcYO02yG56PrRZBYogBh97s49zOc5FbZwQKBgQDXV89KueMsujsYHnm+
SZx/1btH/EWXHzOC5XkMhtK8Y7HrcKFYaRLEAEkBv3xNMiosD0FBwPugsPm4iINT
eBtXGL5JY8ZsAPWtpLZWq9yFzng4lp3n3Vta9IvWIgGDZ2IGgzhq6oFrknTRKes7
VGKgu8VBcNnvP1ysaqywGCb72wKBgQDUmAbzkgQogYPIsQFWRf0w61df7zuoO0/s
xZNsIScSfRhUnfLyBOh5C2IuUnRwtnuF+1HKSmJNo1GhI5hqhj8zJleel3W7+p7e
oBSkILLnP5jbruNZoKEf380jgp8SIk5Ef0lxld0kxOmTfdMsN7I+x/JrajPcGPaP
VSs/1QT+DwKBgQCFag6wkkgv3tVb1Q3CGeMOxEE6kQ4gWaFVWIxNeX44X1/MqUQc
/UQ2EKMqpRMC1LCSCYV5knGTFfIxJMqQPRpbNKY328wD//g185VQTzvZ3phXHuGH
1HmT+WxlZz4exj9SH5wliVJTbjJXoCvv3xEX2h2UtLEg69WjsJd6pgwI/wKBgCIl
IMiyJRTUaHQtacePijD3O5te8zf7/sRKn3j4giwIB4ZfsAuLGkOGvoguGiGYTZKh
YOuasttBZfT5oJtLYI84k04XiYNdp3KeR3JtBg76OfTezAkzMW3LJkmTyzTAac26
m/MwXMpxDgrwZKBveaN3vcnezuGE6OTwive/oQOlAoGAIYJr31wN7rNwXRwmLF5d
f6H8ADdyFPSVfFvLgXl+OX6ZfAdwPIWcz/qJ8bk7ArLUg+cxSOV494K8NPbMfKZP
+jyRqXgv9lRWhXl3x7GJZS3sq9VlKKU7JBAO+tbJkMHfGK23dNhA2CZbeFgK7mfQ
JusDRF34dBKBzJeWwywAvaE=
-----END PRIVATE KEY-----`

var banner = `go-myapps-app-opentalk, Version %s, Open Source Version, made in Hamburg/Germany with <3 by Rico Schulte in 4/2023
`

func main() {
	if version == "" {
		version = "dev"
	}
	fmt.Printf(banner, version)
	flag.Parse()

	initLogging(*log_level)

	if *domain == "" {
		fmt.Println("no or empty domain.")
		os.Exit(1)
	}
	if *name == "" {
		fmt.Println("no or empty name.")
		os.Exit(1)
	}
	if *instance == "" {
		fmt.Println("no or empty instance.")
		os.Exit(1)
	}
	if *password == "" {
		fmt.Println("no or empty password. you have to set a password key to en/decrypt database entries")
		os.Exit(1)
	}

	if *redis_host == "" {
		fmt.Println("no or empty redis_host.")
		os.Exit(1)
	}

	if *redis_port == 0 {
		fmt.Println("no or empty redis_port.")
		os.Exit(1)
	}

	if *tlsCertKeyFile != "" && *tlsCertFile == "" {
		fmt.Println("when using a certificate key from file, you have to set a certificate file too.")
		os.Exit(1)
	} else if *tlsCertKeyFile == "" && *tlsCertFile != "" {
		fmt.Println("when using a certificate from file, you have to set a certificate key file too.")
		os.Exit(1)
	} else if *tlsCertKeyFile != "" && *tlsCertFile != "" {
		certBytes, err := os.ReadFile(*tlsCertFile)
		if err != nil {
			fmt.Printf("error loading certificate from file '%s': %v", *tlsCertFile, err)
			os.Exit(1)
		}
		keyBytes, err := os.ReadFile(*tlsCertKeyFile)
		if err != nil {
			fmt.Printf("error loading certificate key from file '%s': %v", *tlsCertKeyFile, err)
			os.Exit(1)
		}
		certificate = string(certBytes)
		certificateKey = string(keyBytes)
	}

	// use the embedded directory as default
	fsRoot, _ := fs.Sub(StaticFiles, "static")
	fileSystem := http.FS(fsRoot)

	// unless we get a filepath to a directory to serve the files from
	if *staticDir != "" {
		fileSystem = http.Dir(*staticDir)
	}

	// create a instance of our app service
	mService, err := service.NewAppService(
		"0.0.0.0",
		*port_http,
		*port_https,
		certificate,
		certificateKey,
		*domain,
		*name,
		*instance,
		*password,
		fileSystem,
	)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	app, err := NewAppInstance(mService)
	if err != nil {
		fmt.Printf("error creating app instance: %v", err)
		os.Exit(1)
	}
	app.Start()

	pbxApi := pbxapi.NewPbxApi()
	go func() {
		inputchannel := pbxApi.AddReceiver()
		defer pbxApi.RemoveReceiver(inputchannel)

		for msg := range inputchannel {
			switch msg.Type {

			case pbxapi.PbxApiEventConnect:
				log.Debugf("++++++++++++++++ pbxapi.PbxApiEventConnect %+v", msg)

			case pbxapi.PbxApiEventDisconnect:
				log.Debugf("++++++++++++++++ pbxapi.PbxApiEventDisconnect %+v", msg)

			default:
				//log.Debugf("++++++++++++++++ %+v", msg)
			}
		}
	}()
	mService.RegisterHandler(pbxApi)

	pbxTablesUsers := pbxtableusers.NewPbxTableUsers()
	go func() {
		inputchannel := pbxTablesUsers.AddReceiver()
		defer pbxTablesUsers.RemoveReceiver(inputchannel)

		for msg := range inputchannel {
			switch msg.Type {

			case pbxtableusers.PbxTableUsersEventConnect:
				mbytes, _ := json.Marshal(pbxtableusers.NewReplicateStart(
					true,
					true,
					pbxtableusers.AllColumns,
					[]string{"", "executive"},
					strconv.FormatInt(time.Now().UnixNano(), 10),
				))
				msg.Connection.WriteMessage(mbytes)

			case pbxtableusers.PbxTableUsersEventInitialDone:
				for guid, object := range pbxTablesUsers.ReplicatedObjects {
					app.PbxObjectAdd(guid, &object)
				}
				log.Infof("finished syncing of %d users of pbx '%s'", len(pbxTablesUsers.ReplicatedObjects), msg.Connection.PbxInfo.Pbx)

			case pbxtableusers.PbxTableUsersEventAdd:
				app.PbxObjectAdd(msg.Object.Guid, msg.Object)

			case pbxtableusers.PbxTableUsersEventUpdate:
				app.PbxObjectUpdate(msg.Object.Guid, msg.Object)

			case pbxtableusers.PbxTableUsersEventDelete:
				app.PbxObjectDelete(msg.Object.Guid)

			default:
				//log.Debugf("+++++++++++++++++++++++++++++ PbxTableUsersEvent %d", msg.Type)
			}
		}
	}()
	mService.RegisterHandler(pbxTablesUsers)

	// start the app service
	mService.Start()

	// do not exit the program, we have go routines that are running
	select {}
}

func initLogging(level_string string) error {
	// Log as JSON instead of the default ASCII formatter.
	//log.SetFormatter(&log.JSONFormatter{})
	log.SetFormatter(&log.TextFormatter{
		DisableColors: false,
		FullTimestamp: true,
		PadLevelText:  false,
	})
	// Output to stdout instead of the default stderr
	// Can be any io.Writer, see below for File example
	log.SetOutput(os.Stdout)

	// Only log the warning severity or above.

	//log.SetLevel(log.InfoLevel)
	level, err := log.ParseLevel(level_string)
	if err != nil {
		return err
	}
	log.SetLevel(level) //log.TraceLevel)

	return nil
}

func stringInSlice(s string, slice []string) bool {
	for _, str := range slice {
		if str == s {
			return true
		}
	}
	return false
}
