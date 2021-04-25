package mode

import (
	"bytes"
	"fmt"
	"github.com/golangmc/minecraft-server/apis"
	"github.com/golangmc/minecraft-server/impl/conf"

	"github.com/golangmc/minecraft-server/apis/data/chat"
	"github.com/golangmc/minecraft-server/apis/data/msgs"
	"github.com/golangmc/minecraft-server/apis/game"
	"github.com/golangmc/minecraft-server/apis/util"
	"github.com/golangmc/minecraft-server/apis/uuid"
	"github.com/golangmc/minecraft-server/impl/base"
	"github.com/golangmc/minecraft-server/impl/game/auth"
	"github.com/golangmc/minecraft-server/impl/game/ents"
	"github.com/golangmc/minecraft-server/impl/prot/client"
	"github.com/golangmc/minecraft-server/impl/prot/server"
)

/**
 * login
 */

func HandleState2(config *conf.ServerConfig, watcher util.Watcher, join chan base.PlayerAndConnection) {

	watcher.SubAs(func(packet *server.PacketILoginStart, conn base.Connection) {
		playerName := packet.PlayerName

		if !config.OnlineMode {
			playerUuid := uuid.TextToUUID("OfflinePlayer:" + playerName)

			prof := game.Profile{
				UUID: playerUuid,
				Name: playerName,
			}

			login(prof, conn, join)
			return
		}

		conn.CertifyValues(playerName)

		_, public := auth.NewCrypt()

		response := client.PacketOEncryptionRequest{
			Server: "",
			Public: public,
			Verify: conn.CertifyData(),
		}

		conn.SendPacket(&response)
	})

	watcher.SubAs(func(packet *server.PacketIEncryptionResponse, conn base.Connection) {
		defer func() {
			if err := recover(); err != nil {
				conn.SendPacket(&client.PacketODisconnect{
					Reason: *msgs.New(fmt.Sprintf("Authentication failed: %v", err)).SetColor(chat.Red),
				})
			}
		}()

		ver, err := auth.Decrypt(packet.Verify)
		if err != nil {
			panic(fmt.Errorf("failed to decrypt token: %s\n%v\n", conn.CertifyName(), err))
		}

		if !bytes.Equal(ver, conn.CertifyData()) {
			panic(fmt.Errorf("encryption failed, tokens are different: %s\n%v | %v", conn.CertifyName(), ver, conn.CertifyData()))
		}

		sec, err := auth.Decrypt(packet.Secret)
		if err != nil {
			panic(fmt.Errorf("failed to decrypt secret: %s\n%v\n", conn.CertifyName(), err))
		}

		conn.CertifyUpdate(sec) // enable encryption on the connection

		auth.RunAuthGet(sec, conn.CertifyName(), func(auth *auth.Auth, err error) {
			defer func() {
				if err := recover(); err != nil {
					conn.SendPacket(&client.PacketODisconnect{
						Reason: *msgs.New(fmt.Sprintf("Authentication failed: %v", err)).SetColor(chat.Red),
					})
				}
			}()

			if err != nil {
				panic(fmt.Errorf("failed to authenticate: %s\n%v\n", conn.CertifyName(), err))
			}

			playerUuid := uuid.FromString(auth.UUID)

			prof := game.Profile{
				UUID: playerUuid,
				Name: auth.Name,
			}

			for _, prop := range auth.Prop {
				prof.Properties = append(prof.Properties, &game.ProfileProperty{
					Name:      prop.Name,
					Value:     prop.Data,
					Signature: prop.Sign,
				})
			}

			login(prof, conn, join)
		})

	})

}

func login(prof game.Profile, conn base.Connection, join chan base.PlayerAndConnection) {
	s := apis.MinecraftServer()

	p := s.PlayerByUUID(prof.UUID)

	if p != nil {
		conn.SendPacket(&client.PacketODisconnect{
			Reason: *msgs.New(fmt.Sprintf("Player with name \"%s\" already play on the server.", p.Name())).SetColor(chat.Red),
		})
	}

	player := ents.NewPlayer(&prof, conn)

	conn.SendPacket(&client.PacketOLoginSuccess{
		PlayerName: player.Name(),
		PlayerUUID: player.UUID().String(),
	})

	conn.SetState(base.PLAY)

	join <- base.PlayerAndConnection{
		Player:     player,
		Connection: conn,
	}
}
