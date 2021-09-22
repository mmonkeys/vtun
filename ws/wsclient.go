package ws

import (
	"io"
	"log"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/net-byte/vtun/common/cipher"
	"github.com/net-byte/vtun/common/config"
	"github.com/net-byte/vtun/common/netutil"
	"github.com/net-byte/vtun/tun"
	"github.com/patrickmn/go-cache"
	"github.com/songgao/water"
	"github.com/songgao/water/waterutil"
)

// StartClient starts ws client
func StartClient(config config.Config) {
	iface := tun.CreateTun(config)
	c := cache.New(30*time.Minute, 10*time.Minute)
	log.Printf("vtun ws client started,CIDR is %v", config.CIDR)
	// read data from tun
	packet := make([]byte, 1500)
	for {
		n, err := iface.Read(packet)
		if err != nil || n == 0 {
			continue
		}
		b := packet[:n]
		if !waterutil.IsIPv4(b) {
			continue
		}
		srcAddr, dstAddr := netutil.GetAddr(b)
		if srcAddr == "" || dstAddr == "" {
			continue
		}
		key := strings.Join([]string{dstAddr, srcAddr}, "->")
		var conn *websocket.Conn
		v, ok := c.Get(key)
		if ok {
			conn = v.(*websocket.Conn)
		} else {
			conn = netutil.ConnectWS(config)
			if conn == nil {
				continue
			}
			c.Set(key, conn, cache.DefaultExpiration)
			go wsToTun(config, c, key, conn, iface)
		}
		if config.Obfuscate {
			b = cipher.XOR(b)
		}
		conn.WriteMessage(websocket.BinaryMessage, b)
	}
}

func wsToTun(config config.Config, c *cache.Cache, key string, wsConn *websocket.Conn, iface *water.Interface) {
	defer netutil.CloseWS(wsConn)
	for {
		wsConn.SetReadDeadline(time.Now().Add(time.Duration(30) * time.Second))
		_, b, err := wsConn.ReadMessage()
		if err != nil || err == io.EOF {
			break
		}
		if config.Obfuscate {
			b = cipher.XOR(b)
		}
		if !waterutil.IsIPv4(b) {
			continue
		}
		iface.Write(b[:])
	}
	c.Delete(key)
}
