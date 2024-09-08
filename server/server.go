package main

import (
	"fmt"
	"io"
	"log"
	"net"
	_ "net/http/pprof" // 会自动注册 handler 到 http server，方便通过 http 接口获取程序运行采样报告
	"os"
	"time"

	"github.com/brickzzhang/zero-downtime-upgrade/fdtrans"
)

const (
	UnixProto       = "unix"
	BusinessUdsAddr = "/tmp/socket_takeover.sock"
	UpgDataUdsAddr  = "/tmp/upg_data.sock"

	SideServer = "server"
	SideClient = "client"
)

type UpgData struct {
	ln   net.Listener
	conn net.Conn
	side string
}

var upgGlobal UpgData

func startUpgDataServer() (net.Listener, error) {
	ln, err := net.Listen(UnixProto, UpgDataUdsAddr)
	if err != nil {
		log.Printf("listen error: %v", err)
		return nil, err
	}
	return ln, nil
}

func startUpgDataClient() (net.Conn, error) {
	conn, err := net.Dial(UnixProto, UpgDataUdsAddr)
	if err != nil {
		log.Printf("dial error: %v", err)
		return nil, err
	}
	return conn, nil
}

func startUpgDataFlow() error {
	// sock文件存在，以client模式启动
	_, err := os.Stat(UpgDataUdsAddr)
	if err == nil {
		upgGlobal.side = SideClient
		conn, err := startUpgDataClient()
		if err != nil {
			log.Printf("start upg data client error: %v", err)
			return err
		}
		upgGlobal.conn = conn
		return nil
	}

	// sock文件不存在，以server模式首次启动
	if os.IsNotExist(err) {
		upgGlobal.side = SideServer
		ln, err := startUpgDataServer()
		if err != nil {
			log.Printf("start upg data server error: %v", err)
			return err
		}
		upgGlobal.ln = ln
		return nil
	}

	return fmt.Errorf("start upg data flow, unknown error: %+v", err)
}

func main() {
	// start upg data server
	if err := startUpgDataFlow(); err != nil {
		log.Printf("start upg data server error: %v", err)
		panic(err)
	}
	// 以server模式启动，将连接conn发送给unix socket
	if upgGlobal.side == SideServer {
		log.Printf("start as server side")

		ln, err := net.Listen(UnixProto, BusinessUdsAddr)
		if err != nil {
			log.Printf("listen error: %v", err)
			return
		}
		// can't close ln, otherwise, unix socket file will be removed
		// defer ln.Close()
		// 保持对后续连接的接受能力
		lnUnix, _ := ln.(*net.UnixListener)
		lnConn, _ := lnUnix.SyscallConn()
		var lnFd uintptr
		if controlErr := lnConn.Control(func(rawFD uintptr) {
			lnFd = rawFD
		}); controlErr != nil {
			log.Printf("rawconn control failed, err: %+v", controlErr)
			return
		}
		lnFile := os.NewFile(lnFd, "dump_ln")

		for {
			conn, err := ln.Accept()
			if err != nil {
				log.Printf("accept error: %v", err)
				continue
			}
			go func(c net.Conn) {
				for {
					c.SetDeadline(time.Now().Local().Add(10 * time.Second))
					buf := make([]byte, 20)
					_, err := c.Read(buf)
					if err != nil && err != io.EOF {
						c.Close()
						break
					}
					if err == io.EOF {
						break
					}
					// 模拟任务处理耗时
					time.Sleep(2 * time.Second)
					fmt.Println("recv: ", string(buf))
					if _, err = c.Write([]byte(fmt.Sprintf("hello client: %s", string(buf)))); err != nil {
						log.Printf("as server side, write error: %v", err)
					}
				}
				c.Close()
			}(conn)

			// 获取conn对应fd
			unixConn, ok := conn.(*net.UnixConn)
			if !ok {
				log.Printf("conn is not a unix conn")
				return
			}
			// unixConn.File()
			rawConn, err := unixConn.SyscallConn()
			if err != nil {
				log.Printf("get syscall of unix conn err: %+v", err)
				return
			}
			var fd uintptr
			if controlErr := rawConn.Control(func(rawFD uintptr) {
				fd = rawFD
			}); controlErr != nil {
				log.Printf("rawconn control failed, err: %+v", controlErr)
				return
			}
			dumpFile := os.NewFile(fd, "dump_ln")

			// 将fd发送到unix socket
			a, err := upgGlobal.ln.Accept()
			defer a.Close()
			upgDataConn := a.(*net.UnixConn)
			if err = fdtrans.Put(upgDataConn, lnFile, dumpFile); err != nil {
				log.Printf("put fd error: %v", err)
				return
			}

			// 关闭conn
			conn.Close()
			log.Printf("conn closed, as server side")
			break
		}
	} else {
		log.Printf("start as client side")

		// 提取收到的fd
		defer upgGlobal.conn.Close()
		fdConn := upgGlobal.conn.(*net.UnixConn)
		fds, err := fdtrans.Get(fdConn, 1, []string{"a file"})
		if err != nil {
			log.Printf("get fd error: %v", err)
			return
		}

		// get listener fd
		lnFd := fds[0]
		// defer lnFd.Close()
		ln, err := net.FileListener(lnFd)
		if err != nil {
			log.Printf("file listener error, as client side: %v", err)
			return
		}

		// get existing conn fd
		connFd := fds[1]
		defer connFd.Close()

		// 处理存量conn
		go func() {
			for {
				buf := make([]byte, 20)
				_, err := connFd.Read(buf)
				if err != nil && err != io.EOF {
					log.Printf("read error: %v, as client side", err)
					connFd.Close()
					break
				}
				if err == io.EOF {
					log.Printf("eof recieved, as client side")
					break
				}
				// 模拟任务处理耗时
				time.Sleep(2 * time.Second)
				fmt.Println("recv: ", string(buf))
				connFd.Write([]byte(fmt.Sprintf("hello client, existing conn: %s", string(buf))))
			}
		}()
		// 处理新conn
		for {
			conn, err := ln.Accept()
			if err != nil {
				log.Printf("accept error: %v", err)
				continue
			}
			go func(c net.Conn) {
				for {
					c.SetDeadline(time.Now().Local().Add(10 * time.Second))
					buf := make([]byte, 20)
					_, err := c.Read(buf)
					if err != nil && err != io.EOF {
						c.Close()
						break
					}
					if err == io.EOF {
						break
					}
					// 模拟任务处理耗时
					time.Sleep(2 * time.Second)
					fmt.Println("recv: ", string(buf))
					if _, err = c.Write([]byte(fmt.Sprintf("hello client, new conn: %s", string(buf)))); err != nil {
						log.Printf("as client side, new conn, write error: %v", err)
					}
				}
				c.Close()
			}(conn)
		}
	}
}
