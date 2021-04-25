package impl

import (
	"fmt"
	"github.com/golangmc/minecraft-server/apis/data"
	apis_level "github.com/golangmc/minecraft-server/apis/game/level"
	impl_level "github.com/golangmc/minecraft-server/impl/game/level"
	client_packet "github.com/golangmc/minecraft-server/impl/prot/client"
	"github.com/golangmc/minecraft-server/lib"
	"strconv"
	"strings"
	"time"

	"github.com/golangmc/minecraft-server/apis"
	"github.com/golangmc/minecraft-server/apis/cmds"
	"github.com/golangmc/minecraft-server/apis/data/chat"
	"github.com/golangmc/minecraft-server/apis/ents"
	"github.com/golangmc/minecraft-server/apis/logs"
	"github.com/golangmc/minecraft-server/apis/task"
	"github.com/golangmc/minecraft-server/apis/util"
	"github.com/golangmc/minecraft-server/apis/uuid"
	"github.com/golangmc/minecraft-server/impl/conf"
	"github.com/golangmc/minecraft-server/impl/data/plugin"

	"github.com/golangmc/minecraft-server/impl/conn"
	"github.com/golangmc/minecraft-server/impl/cons"
	"github.com/golangmc/minecraft-server/impl/data/system"
	"github.com/golangmc/minecraft-server/impl/data/values"
	"github.com/golangmc/minecraft-server/impl/prot"

	apis_base "github.com/golangmc/minecraft-server/apis/base"
	impl_base "github.com/golangmc/minecraft-server/impl/base"

	apis_event "github.com/golangmc/minecraft-server/apis/game/event"
	impl_event "github.com/golangmc/minecraft-server/impl/game/event"
)

type Properties struct {
	OnlineMode bool
}

type server struct {
	message chan system.Message

	console *cons.Console

	logging *logs.Logging
	tasking *task.Tasking
	watcher util.Watcher

	command *cmds.CommandManager

	network impl_base.Network
	packets impl_base.Packets

	players *playerAssociation

	config *conf.ServerConfig

	level  apis_level.Level
}

// NewServer ==== new ====
func NewServer(conf *conf.ServerConfig) apis.Server {
	message := make(chan system.Message)

	console := cons.NewConsole(message)

	logging := logs.NewLogging("server", logs.EveryLevel...)
	tasking := task.NewTasking(values.MPT)
	watcher := util.NewWatcher()

	join := make(chan impl_base.PlayerAndConnection)
	quit := make(chan impl_base.PlayerAndConnection)

	packets := prot.NewPackets(conf, tasking, join, quit)
	network := conn.NewNetwork(conf.Network.Host, conf.Network.Port, packets, message, join, quit)

	command := cmds.NewCommandManager()

	return &server{
		message: message,

		console: console,

		logging: logging,
		tasking: tasking,
		watcher: watcher,

		command: command,

		packets: packets,
		network: network,

		config: conf,

		players: &playerAssociation{
			uuidToData: make(map[uuid.UUID]ents.Player),

			connToUUID: make(map[impl_base.Connection]uuid.UUID),
			uuidToConn: make(map[uuid.UUID]impl_base.Connection),
		},
	}
}

// Load ==== State ====
func (s *server) Load() {
	apis.SetMinecraftServer(s)

	go s.loadWorld()
	go s.loadServer()
	go s.readInputs()

	s.wait()
}

func (s *server) Kill() {
	lib.ReadLine().Close()

	s.console.Kill()
	s.command.Kill()
	s.tasking.Kill()
	s.network.Kill()

	// push the stop message to the server exit channel
	s.message <- system.Make(system.STOP, "normal stop")
	close(s.message)

	s.logging.Info(chat.DarkRed, "Server will be stopped")
}

// Logging ==== Server ====
func (s *server) Logging() *logs.Logging {
	return s.logging
}

func (s *server) Command() *cmds.CommandManager {
	return s.command
}

func (s *server) Tasking() *task.Tasking {
	return s.tasking
}

func (s *server) Watcher() util.Watcher {
	return s.watcher
}

func (s *server) Players() []ents.Player {
	players := make([]ents.Player, 0)

	for _, player := range s.players.uuidToData {
		players = append(players, player)
	}

	return players
}

func (s *server) ConnByUUID(uuid uuid.UUID) impl_base.Connection {
	return s.players.uuidToConn[uuid]
}

func (s *server) PlayerByUUID(uuid uuid.UUID) ents.Player {
	return s.players.uuidToData[uuid]
}

func (s *server) PlayerByConn(conn impl_base.Connection) ents.Player {
	uuid, con := s.players.connToUUID[conn]
	if !con {
		return nil
	}

	return s.PlayerByUUID(uuid)
}

func (s *server) ServerVersion() string {
	return "0.0.1-SNAPSHOT"
}

func (s *server) Broadcast(message string) {
	s.console.SendMessage(message)

	for _, player := range s.Players() {
		player.SendMessage(message)
	}
}

// ==== server commands ====
func (s *server) broadcastCommand(sender ents.Sender, params []string) {
	message := strings.Join(params, " ")

	for _, player := range s.Players() {
		player.SendMessage(message)
	}
}

func (s *server) stopServerCommand(sender ents.Sender, params []string) {
	if _, ok := sender.(*cons.Console); !ok {
		s.logging.FailF("non console sender %s tried to stop the server", sender.Name())
		return
	}

	var after int64 = 0

	if len(params) > 0 {
		param, err := strconv.Atoi(params[0])

		if err != nil {
			panic(err)
		}

		if param <= 0 {
			panic(fmt.Errorf("value must be a positive whole number. [1..]"))
		}

		after = int64(param)
	}

	if after == 0 {

		s.Kill()

	} else {

		// inform future shutdown
		s.logging.Warn(chat.Gold, "stopping server in ", chat.Green, util.FormatTime(after))

		// schedule shutdown {after} seconds later
		s.tasking.AfterTime(after, time.Second, func(task *task.Task) {
			s.Kill()
		})

	}
}

func (s *server) setBlockCommand(sender ents.Sender, params []string) {
	if _, ok := sender.(*cons.Console); ok {
		sender.SendMessage("Sorry but you can't execute this command!")
		return
	}

	player := s.PlayerByUUID(sender.UUID())
	loc := player.GetLocation()

	x := int(loc.X)
	y := int(loc.Y)
	z := int(loc.Z)

	value, _ := strconv.ParseInt(params[0], 10, 16)

	block := s.GetLevel().GetBlock(x, y, z)
	block.SetBlockType(int(value))

	sender.SendMessage("Trying to set block around you.")

	conn := s.players.uuidToConn[sender.UUID()]
	for _, chunk := range s.GetLevel().Chunks() {
		conn.SendPacket(&client_packet.PacketOChunkData{Chunk: chunk})
	}
}

func (s *server) teleportCommand(sender ents.Sender, params []string) {
	if _, ok := sender.(*cons.Console); ok {
		sender.SendMessage(chat.Translate("&cOnly user can run this command."))
		return
	}

	if 3 > len(params) {
		sender.SendMessage(chat.Translate("&cPlease use example: /tp [x] [y] [z]"))
		return
	}

	x, _ := strconv.ParseFloat(params[0], 64)
	y, _ := strconv.ParseFloat(params[1], 64)
	z, _ := strconv.ParseFloat(params[2], 64)

	newLoc := data.Location{
		PositionF: data.PositionF{
			X: x,
			Y: y,
			Z: z,
		},
	}

	sender.SendMessage("Trying to teleport you.")

	conn := s.ConnByUUID(sender.UUID())
	conn.SendPacket(&client_packet.PacketOPlayerLocation{Location: newLoc})
}

func (s *server) versionCommand(sender ents.Sender, params []string) {
	sender.SendMessage(s.ServerVersion())
}

// ==== internal ====
func (s *server) loadServer() {
	s.console.Load()
	s.command.Load()
	s.tasking.Load()
	s.network.Load()

	s.logRunningStatus()

	s.command.Register("vers", s.versionCommand)
	s.command.Register("send", s.broadcastCommand)
	s.command.Register("stop", s.stopServerCommand)
	s.command.Register("tp", s.teleportCommand)
	s.command.Register("setblock", s.setBlockCommand)

	s.watcher.SubAs(func(event apis_event.PlayerJoinEvent) {
		s.logging.InfoF("player %s logged in with uuid:%v", event.Player.Name(), event.Player.UUID())

		s.Broadcast(chat.Translate(fmt.Sprintf("%s%s has joined!", chat.Yellow, event.Player.Name())))
	})
	s.watcher.SubAs(func(event apis_event.PlayerQuitEvent) {
		s.logging.InfoF("%s disconnected!", event.Player.Name())

		s.Broadcast(chat.Translate(fmt.Sprintf("%s%s has left!", chat.Yellow, event.Player.Name())))
	})

	s.watcher.SubAs(func(event impl_event.PlayerConnJoinEvent) {
		s.players.addData(event.Conn)

		s.watcher.PubAs(apis_event.PlayerJoinEvent{PlayerEvent: apis_event.PlayerEvent{Player: event.Conn.Player}})
	})
	s.watcher.SubAs(func(event impl_event.PlayerConnQuitEvent) {
		player := s.players.playerByConn(event.Conn.Connection)

		if player != nil {
			s.watcher.PubAs(apis_event.PlayerQuitEvent{PlayerEvent: apis_event.PlayerEvent{Player: player}})
		}

		s.players.delData(event.Conn)
	})

	s.watcher.SubAs(func(event impl_event.PlayerPluginMessagePullEvent) {
		s.logging.DataF("received message on channel '%s' from player %s:%s", event.Channel, event.Conn.Name(), event.Conn.UUID())

		switch event.Channel {
		case plugin.CHANNEL_BRAND:
			s.logging.DataF("their client's brand is '%s'", event.Message.(*plugin.Brand).Name)
		}
	})
}

func (s *server) logRunningStatus() {
	mode := "Offline"

	if s.config.OnlineMode {
		mode = "Online"
	}

	s.logging.InfoF("running in %s mode", mode)
}

func (s *server) readInputs() {
	for {
		// read input from console
		text := strings.Trim(<-s.console.IChannel, " ")
		if len(text) == 0 {
			continue
		}

		args := strings.Split(text, " ")
		if len(args) == 0 {
			continue
		}

		if command := s.command.Search(args[0]); command != nil {

			err := apis_base.Attempt(func() {
				(*command).Evaluate(s.console, args[1:])
			})

			if err != nil {
				s.logging.Fail(
					chat.Red, "failed to evaluate ",
					chat.DarkGray, "`",
					chat.White, (*command).Name(),
					chat.DarkGray, "`",
					chat.Red, ": ", err.Error()[8:])
			}

			continue
		}

		s.console.SendMessage(text)
	}
}

func (s *server) wait() {
	// select over server commands channel
	select {
	case command := <-s.message:
		switch command.Command {
		// stop selecting when stop is received
		case system.STOP:
			return
		case system.FAIL:
			s.logging.Fail("internal server error: ", command.Message)
			s.logging.Fail("stopping server")
			return
		}
	}

	s.wait()
}

func (s *server) loadWorld() {
	level := impl_level.NewLevel("world")
	impl_level.GenSuperFlat(level, 6)

	s.level = level
}

func (s *server) GetLevel() apis_level.Level {
	return s.level
}

// ==== players ====
type playerAssociation struct {
	uuidToData map[uuid.UUID]ents.Player

	connToUUID map[impl_base.Connection]uuid.UUID
	uuidToConn map[uuid.UUID]impl_base.Connection
}

func (p *playerAssociation) addData(data impl_base.PlayerAndConnection) {
	p.uuidToData[data.Player.UUID()] = data.Player

	p.connToUUID[data.Connection] = data.Player.UUID()
	p.uuidToConn[data.Player.UUID()] = data.Connection
}

func (p *playerAssociation) delData(data impl_base.PlayerAndConnection) {
	player := p.playerByConn(data.Connection)

	uuid := p.connToUUID[data.Connection]

	delete(p.connToUUID, data.Connection)
	delete(p.uuidToConn, uuid)

	if player != nil {
		delete(p.uuidToData, player.UUID())
	}
}

func (p *playerAssociation) playerByUUID(uuid uuid.UUID) ents.Player {
	return p.uuidToData[uuid]
}

func (p *playerAssociation) playerByConn(conn impl_base.Connection) ents.Player {
	uuid, con := p.connToUUID[conn]

	if !con {
		return nil
	}

	data, con := p.uuidToData[uuid]

	if !con {
		return nil
	}

	return data
}