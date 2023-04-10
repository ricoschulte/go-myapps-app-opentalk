package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	"github.com/ricoschulte/go-myapps/service"
	"github.com/ricoschulte/go-myapps/service/pbxapi"
	"github.com/ricoschulte/go-myapps/service/pbxtableusers"
	log "github.com/sirupsen/logrus"
)

const OpenTalkEventJoined = 1
const OpenTalkEventLeft = 2

// format parser of events of the redis publish messages
type RedisEvent struct {
	Room        string `json:"room"`
	Participant string `json:"participant"`
	Event       string `json:"event"`
}

func (revt *RedisEvent) GetType() (int, error) {
	switch revt.Event {
	default:
		return 0, fmt.Errorf("unknown RedisEvent: %s", revt.Event)
	case "joined":
		return OpenTalkEventJoined, nil
	case "left":
		return OpenTalkEventLeft, nil
	}
}

type OpenTalkEvent struct {
	Type        int // joined / left
	Room        string
	User        string
	Email       string
	Participant string
}

// Define a function type for the event callback
type EventHandler func(evt *OpenTalkEvent)

type App struct {
	AppService *service.AppService
	Context    context.Context
	PbxUsers   map[string]pbxtableusers.ReplicatedObject
}

func NewAppInstance(appservice *service.AppService) (*App, error) {
	instance := App{
		Context:    context.Background(),
		AppService: appservice,
		PbxUsers:   map[string]pbxtableusers.ReplicatedObject{},
	}
	return &instance, nil
}

func (app *App) Start() error {

	go func() {
		for {
			app.ListenForUserJoinLeaveEvents()
			time.Sleep(1 * time.Second)
		}
	}()
	return nil
}

func (app *App) PbxObjectAdd(guid string, object *pbxtableusers.ReplicatedObject) {
	log.
		WithField("Guid", object.Guid).
		WithField("Emails", object.Emails).
		Debug("PbxObjectAdd")
	app.PbxUsers[guid] = *object
}

func (app *App) PbxObjectUpdate(guid string, object *pbxtableusers.ReplicatedObject) {
	app.PbxUsers[guid] = *object
}

func (app *App) PbxObjectDelete(guid string) {
	delete(app.PbxUsers, guid)
}

// searches a PbxObject of a user with the searched email
// - either there is a object that has a email that matches exacly the given email.
// - or when a email field in the pbx does not have a domain part (@company.com), than the object is returned when the email matches entry@pbxdomain.
// when no user with that email is found a error is returned
func (app *App) PbxGetObjectByEmail(pbxdomain string, email string) (*pbxtableusers.ReplicatedObject, error) {
	log.
		WithField("num_synced_pbx_users", len(app.PbxUsers)).
		Debugf("searching for pbx object with email '%s'", email)
	for _, object := range app.PbxUsers {
		for _, objemail := range object.Emails {
			// exact match
			if email == objemail.Email {
				return &object, nil
			}

			// if the objects email does not have a @ we test, if the domain of the pbx matches
			if !strings.Contains(objemail.Email, "@") {
				if pbxdomain == strings.Split(email, "@")[1] && email == fmt.Sprintf("%s@%s", objemail.Email, pbxdomain) {
					return &object, nil
				}
			}
		}
	}
	return nil, errors.New("no object with email found")
}

// called when a user joined or left a room in opentalk.
// does the change of the presence at the pbx
func (app *App) OnOpentalkEvent(evt *OpenTalkEvent) {
	log.
		WithField("Type", evt.Type).
		WithField("Room", evt.Room).
		WithField("User", evt.User).
		WithField("Email", evt.Email).
		WithField("Participant", evt.Participant).
		Debug("onOpentalkEvent")

	// get the systemdomain of the pbx environment
	// we use the first connection of a pbx for that
	if len(app.AppService.Connections) < 1 {
		log.Warn("no connection of a pbx to get the systemdomain")
		return
	}

	pbxdomain := app.AppService.Connections[0].PbxInfo.Domain
	if pbxdomain == "" {
		log.Warn("the system domain of the pbx is empty. which is maybe a problem")
	}

	pbxobject, err := app.PbxGetObjectByEmail(pbxdomain, evt.Email)
	if err != nil {
		log.
			WithField("Type", evt.Type).
			WithField("Room", evt.Room).
			WithField("User", evt.User).
			WithField("Email", evt.Email).
			WithField("Participant", evt.Participant).
			WithField("pbxdomain", pbxdomain).
			Warnf("user not found in pbx with email: %s", evt.Email)
	} else {

		log.
			WithField("Type", evt.Type).
			WithField("Room", evt.Room).
			WithField("User", evt.User).
			WithField("Email", evt.Email).
			WithField("Participant", evt.Participant).
			WithField("Guid", pbxobject.Guid).
			WithField("H323", pbxobject.H323).
			WithField("pbxdomain", pbxdomain).
			Info("user found in pbx with email")

		if evt.Type == OpenTalkEventJoined {
			app.PbxSetPresence(pbxobject, pbxapi.ActivityBusy, pbx_presence_note)
		} else {
			app.PbxSetPresence(pbxobject, pbxapi.ActivityAvaiable, "")
		}
	}
}

func (app *App) PbxGetPbxConnection(pbxname string) (*service.AppServicePbxConnection, error) {
	for _, conn := range app.AppService.Connections {
		if conn.PbxInfo.Pbx == pbxname {
			return conn, nil
		}
	}
	return nil, fmt.Errorf("no connection of the pbx with name '%s' connected", pbxname)
}

func (app *App) PbxSetPresence(pbxobject *pbxtableusers.ReplicatedObject, activity string, note string) error {
	log.
		WithField("Guid", pbxobject.Guid).
		WithField("H323", pbxobject.H323).
		WithField("loc", pbxobject.Loc).
		WithField("activity", activity).
		WithField("note", note).
		Debug("PbxSetPresence")

	conn, err := app.PbxGetPbxConnection(pbxobject.Loc)
	if err != nil {
		return err
	} else if !stringInSlice("PbxApi", conn.PbxInfo.Apis) {
		return fmt.Errorf("found a connection of the pbx of the user, but the PbxApi is not avaiable. %s - %p", conn.PbxInfo.Pbx, conn.PbxInfo.Apis)
	}
	mbytes, _ := json.Marshal(
		pbxapi.NewSetPresenceWithGuid(
			pbxobject.Guid,
			"tel:",
			activity,
			note,
			strconv.FormatInt(time.Now().UnixNano(), 10),
		),
	)
	return conn.WriteMessage(mbytes)
}

func (app *App) ListenForUserJoinLeaveEvents() {
	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", *redis_host, *redis_port),
		Username: *redis_username,
		Password: *redis_password,
		DB:       *redis_db,
	})
	defer client.Close()

	pubsub := client.PSubscribe(app.Context)
	pubsub.PSubscribe(app.Context, redis_listen_topic)

	ch := pubsub.Channel()
	for msg := range ch {
		log.Debugf("message from redis: %s", msg.Payload)

		// try to parse the message payload as json
		message := RedisEvent{}
		err_parse := json.Unmarshal([]byte(msg.Payload), &message)
		if err_parse != nil {
			log.Errorf("error while parsing event from redis: %p", &err_parse)
			continue
		}

		if message.Event == "" {
			log.Errorf("redis message doesnt have a Event: %s", msg.Payload)
			continue
		}

		if message.Room == "" {
			log.Errorf("redis message doesnt have a Room: %s", msg.Payload)
			continue
		}

		if message.Participant == "" {
			log.Errorf("redis message doesnt have a Participant: %s", msg.Payload)
			continue
		}

		evttype, err_tpye := message.GetType()
		if err_tpye != nil {
			log.Error("error while get type of event from redis message:", err_tpye)
			continue
		}

		// lookup the userid of a Participant via Redis to a opentalk userid
		userid, err := app.GetOpentalkUseridFromParticipantid(message.Room, message.Participant)
		if err != nil {
			log.Error("error while LookupUseridFromParticipant:", err)
			continue
		}

		// resolve the userid to a email via the postgres db
		email, err_email := app.GetOpentalkUserEmail(userid)
		if err != nil {
			log.Error("error while GetOtUserEmail:", err_email)
		}

		// start the handling of the event at the pbx
		app.OnOpentalkEvent(&OpenTalkEvent{
			Type:        evttype,
			Room:        message.Room,
			User:        userid,
			Email:       email,
			Participant: message.Participant,
		})
	}
}

// resolves the userid of a participant in a room via redis
func (app *App) GetOpentalkUseridFromParticipantid(roomid string, participantid string) (string, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", *redis_host, *redis_port),
		Username: *redis_username,
		Password: *redis_password,
		DB:       *redis_db,
	})
	defer client.Close()

	rstring := fmt.Sprintf("k3k-signaling:room=%s:participants:attributes:user_id", roomid)
	userid, err := client.HGet(app.Context, rstring, participantid).Result()
	if err != nil {
		return "", err
	}
	return userid, nil
}

// returns the email of a user from the opentalk postgres
func (app *App) GetOpentalkUserEmail(userid string) (string, error) {
	// Open a connection to the database
	db, err_connect := sql.Open("postgres", fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		*pg_host, *pg_port, *pg_user, *pg_password, *pg_database,
	))
	if err_connect != nil {
		return "", err_connect
	}
	defer db.Close()

	// prepare the query with a parameter
	query := "SELECT email FROM users WHERE id = $1"
	row := db.QueryRow(query, userid)

	// scan the result
	var email string
	err := row.Scan(&email)
	if err != nil {
		return "", err
	}
	return email, nil
}
