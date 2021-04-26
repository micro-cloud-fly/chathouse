package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

// IdleTime 允许用户休眠的时间，即过了这个时间如果一直未发言，则认为要主动踢出这个用户
var IdleTime = 120 * time.Second

//设置踢出用户组的锁，针对map操作
var mapLock sync.RWMutex

//当前服务器启动时允许的默认用户名
var nicknameMap = make(map[string]string)

//此处定义各种输出的颜色效果
var (
	greenBg      = string([]byte{27, 91, 57, 55, 59, 52, 50, 109})
	whiteBg      = string([]byte{27, 91, 57, 48, 59, 52, 55, 109})
	yellowBg     = string([]byte{27, 91, 57, 48, 59, 52, 51, 109})
	redBg        = string([]byte{27, 91, 57, 55, 59, 52, 49, 109})
	blueBg       = string([]byte{27, 91, 57, 55, 59, 52, 52, 109})
	magentaBg    = string([]byte{27, 91, 57, 55, 59, 52, 53, 109})
	cyanBg       = string([]byte{27, 91, 57, 55, 59, 52, 54, 109})
	green        = string([]byte{27, 91, 51, 50, 109})
	white        = string([]byte{27, 91, 51, 55, 109})
	yellow       = string([]byte{27, 91, 51, 51, 109})
	red          = string([]byte{27, 91, 51, 49, 109})
	blue         = string([]byte{27, 91, 51, 52, 109})
	magenta      = string([]byte{27, 91, 51, 53, 109})
	cyan         = string([]byte{27, 91, 51, 54, 109})
	reset        = string([]byte{27, 91, 48, 109})
	disableColor = false
)

// User 用户结构体
type User struct {
	id string
	//用户的姓名
	Name string
	//用户的消息管道，此管道用来接受服务器发来的消息
	MsgChannel chan string
}

//用来存放当前在线所有的用户
var usersMap = make(map[string]*User)

//定义一个全局的管道，用来存放用户发送的消息
var globalMsgChannel = make(chan string, 4)

//存放所有用户的切片，动态监控用户动态
var usersSlice []string

//这个协程主要用来监视切片里面还有几个成员，已经下线的用户，则从切片中删除，删除之后，重新合并切片，每次都是在首的用户是管理员
func electAdmin() {
	for {
		//遍历这个数组
		for i, u := range usersSlice {
			flag := false
			//遍历所有真实存在的用户
			for _, umap := range usersMap {
				//此时表示这个用户还真实存在
				if u == umap.Name {
					flag = true
					break
				}
			}
			//如果这个用户不存在，那么把这个用户踢出，然后进入下轮循环
			if !flag {
				usersSlice = append(usersSlice[:i], usersSlice[i+1:]...)
			}
		}
		time.Sleep(1 * time.Second)
	}
}

//广播消息
func broadcast() {
	for msg := range globalMsgChannel {
		//从全局通道读取到到的数据，广播给每个用户
		//遍历所有的用户
		for s := range usersMap {
			user := usersMap[s]
			channel := user.MsgChannel
			compile := regexp.MustCompile(`【.+】`)
			if compile != nil {
				submatch := compile.FindAllStringSubmatch(msg, 1)
				if len(submatch) == 1 && len(submatch[0]) == 1 {
					username := submatch[0][0]
					username = strings.TrimLeft(username, "【")
					username = strings.TrimRight(username, "】")
					if username == user.Name {
						//此时更改消息标识为是自己发送的内容
						runes := []rune(msg)
						var sss []string
						flag := false
						for _, r := range runes {
							if string(r) == "】" {
								flag = true
								continue
							}
							if flag {
								sss = append(sss, string(r))
							}
						}
						channel <- yellow + "【" + username + "】我" + reset + strings.Join(sss, "") + "\n"
						continue
					}
				}
			}
			channel <- msg + "\n"
		}
	}
}
func main() {
	//监控人员变化
	go electAdmin()
	//启动服务器
	listen, err := net.Listen("tcp", ":9090")
	if err != nil {
		fmt.Println("net.Listen error:", err)
		return
	}
	fmt.Println("服务器启动成功")
	//这个协程，用来遍历收到的用户消息，然后发送给每一个用户
	go broadcast()
	for {
		conn, err := listen.Accept()
		if err != nil {
			fmt.Println("listen.Accept error:", err)
			break
		}
		//每次新建一个链接，都把这个用户放进全局都map中，并且为其分配一个通道，以便后续用来接受消息
		if conn == nil {
			panic("conn为nil")
		}
		userId := conn.RemoteAddr().String()
		//用户进来后，随机给取一个名字，并且把这个名字从map中删除，以防止网名重复
		var nameSlice []string
		for s := range nicknameMap {
			nameSlice = append(nameSlice, s)
		}
		if len(nameSlice) == 0 {
			panic("昵称集合为空")
		}
		randName := nameSlice[rand.Intn(len(nameSlice))]
		user := User{
			id:         userId,
			Name:       randName,
			MsgChannel: make(chan string, 10),
		}
		//此处必须传指针，因为后面有改名字操作
		usersMap[userId] = &user
		//用过了这个名字，就删除
		mapLock.Lock()
		delete(nicknameMap, randName)
		mapLock.Unlock()
		//上线消息写入全局消息管道
		globalMsgChannel <- fmt.Sprintf("【%s】上线了", randName)
		//监听每个用户的动态
		go Handler(conn, &user)
	}
}

// Handler 处理每个用户操作
func Handler(conn net.Conn, user *User) {
	//将所有用户放入切片，切片的第一个用户就是管理员
	usersSlice = append(usersSlice, user.Name)
	//如果这个用户1分钟没有发言，则被踢出聊天室
	var isActive = make(chan bool)
	//是否有被踢出信号
	var isKill = make(chan bool)
	//每个用户对应一个协程，处理每个用户的状态，此处主要用来监控用户的活跃状态
	go func() {
		for {
			select {
			//如果能取到值，那么计时器应该重新计时，此时，走不到上面的倒计时，计时器的管道一直处于阻塞状态
			case <-isKill:
				<-time.After(5 * time.Second)
				//只要能取出时间，就关闭链接
				err := conn.Close()
				if err != nil {
					return
				}
				return
			case <-time.After(IdleTime):
				err := conn.Close()
				if err != nil {
					return
				}
				return
			case <-isActive:
			}

		}
	}()

	//不断的监听用户发来的数据
	go func() {
		for {
			buf := make([]byte, 1024)
			read, err := conn.Read(buf)
			if err != nil {
				fmt.Println("conn.Read error:", err)
				//如果读取的数据出现error,也是认为用户主动退出了
				//if err == io.EOF {
				if err != nil {
					//把这个用户发送的消息放进了全局的通道之中
					globalMsgChannel <- fmt.Sprintf("【%s】下线了", user.Name)
					mapLock.Lock()
					delete(usersMap, user.id)
					mapLock.Unlock()
					//用户下线之后，需要把这个昵称回收
					nicknameMap[user.Name] = user.Name
				}
				return
			}
			//只要读取到数据，就表示用户还处于活跃状态
			isActive <- true
			//这是收到的用户发送的数据,去除最后的回车
			msg := string(buf[:read-1])
			if len(msg) == 5 && msg == "users" {
				//用户想查询当前在线的用户
				var userList []string
				for _, tempUser := range usersMap {
					if tempUser.Name != user.Name {
						userList = append(userList, tempUser.Name)
					}
				}
				//拼接效果如下
				//
				//>管理员
				//   戏志才
				//>我
				//   颜良
				//>其他用户
				//   戏志才
				//   丁廙
				//   张横
				userInfo := greenBg + ">管理员   " + reset + "\n " +
					red + usersSlice[0] + "\n" +
					greenBg + ">我      " + reset + "\n " +
					redBg + user.Name + reset +
					"\n" + greenBg + ">其他用户" + reset + "\n " +
					strings.Join(userList, "\n ")
				//将字符串信息，写给自己，那么可以使用conn直接write回给自己，也可以发送到自己的channel中
				_, err2 := conn.Write([]byte(userInfo + "\n\n"))
				if err2 != nil {
					fmt.Println("users error", err2)
					continue
				}
				continue
			} else if strings.HasPrefix(msg, "rename[") && strings.HasSuffix(msg, "]") && len(msg) > 8 {
				//此处认为是改名
				user.Name = msg[7 : len(msg)-1]
				//改成名字之后，通知这个用户改名成功
				_, _ = conn.Write([]byte(  "修改昵称成功\n"))
				continue

			} else if strings.HasPrefix(msg, "@@") && len(msg) > 4 {
				//此处认为是私信某个用户，只有被私信的用户才能看到这个消息
				compile, err := regexp.Compile("@@.+\\s")
				compile2, err2 := regexp.Compile("\\s.+")
				if err != nil || err2 != nil {
					fmt.Println(err)
					continue
				}
				prefix := compile.FindAllStringSubmatch(msg, 1)
				suffix := compile2.FindAllStringSubmatch(msg, 1)
				tmpUsername := strings.TrimSpace(prefix[0][0])
				tmpUsername = strings.TrimLeft(tmpUsername, "@@")
				secretMsg := strings.TrimSpace(suffix[0][0])
				//获取处被艾特的人之后，需要给这个人发送单独发送数据
				flag := false
				for _, u := range usersMap {
					if tmpUsername == u.Name {
						//找到这个用户之后，向这个用户单独发送数据
						u.MsgChannel <- fmt.Sprintf("【%s】%s@我%s：%s%s%s\n", user.Name, redBg, reset, green, secretMsg, reset)
						flag = true
						break
					}
				}
				if flag {
					continue
				}

			} else if strings.HasPrefix(msg, "tt") && strings.HasSuffix(msg, "tt") {
				//如果发出的信号是踢出信号，则判断这个用户是否是管理员
				//再来判断被踢出的用户是否还存在，如果存在则执行踢出操作
				//假设被踢出的用户是xxx，需要通知用户你即将被踢出下线
				if len(usersSlice) > 0 && user.Name == usersSlice[0] {
					member := getMember(msg)
					if member != "" {
						flag := false
						for _, s := range usersSlice {
							if member == s {
								flag = true
								break
							}
						}
						if flag {
							//向全局管道发送消息
							globalMsgChannel <- fmt.Sprintf("%s将被踢出聊天室", member)
							continue
						}
					}
				}
			} else if (len(strings.TrimSpace(msg))) <= 0 {
				//发的是空消息则不作任何处理
				continue
			}
			//把这个用户发送的消息放进了全局的通道之中
			globalMsgChannel <- fmt.Sprintf("【%s】：%s", user.Name, msg)
		}
	}()
	//不断的监听用户的那个通道里面，有没有消息，如果有，则把消息发送给这个用户
	go func() {
		channel := user.MsgChannel
		for s := range channel {
			//如果消息里面有@用户名，则替换为@我
			//将后面的信息变为绿色
			if strings.Contains(s, "@"+user.Name) {
				split := strings.Split(s, "@"+user.Name)
				s = split[0] + redBg + "@我" + reset + green + split[1] + reset
			} else if strings.Contains(s, user.Name+"将被踢出聊天室") {
				_, _ = conn.Write([]byte("5秒后你将被踢出聊天室...\n"))
				//被踢出
				isKill <- true
				//此处设置类似进度条的功能，只不过此处的目的是想让进度条有中倒退的感觉
				str := "##################################################"
				for i := 50; i >= 0; i-- {
					runes := []rune(str)
					temp := string(runes[:i])
					for j := 0; j < 50-i; j++ {
						temp += "<"
					}
					_, _ = conn.Write([]byte(temp + "\r"))
					time.Sleep(100 * time.Millisecond)
				}
			} else {
				//改变一些输出效果
				if strings.Contains(s, "】：") {
					split := strings.Split(s, "：")
					s = cyan + split[0] + reset + green + strings.TrimSpace(split[1]) + reset + "\n"
				}
			}
			_, _ = conn.Write([]byte(s))
		}
	}()
}

//将所有的网名写入到map中
func init() {
	path := "/Users/java0904/goProject/chathouse/json"
	dir, err := ioutil.ReadDir(path)
	if err != nil {
		panic(err)
	}
	rand.Seed(time.Now().Unix())
	info := dir[rand.Intn(len(dir))]
	file, err := os.Open(path + "/" + info.Name())
	if err != nil {
		panic(err)
	}
	defer file.Close()
	all, err := ioutil.ReadAll(file)
	if err != nil {
		panic(err)
	}
	var names []string
	err = json.Unmarshal(all, &names)
	if err != nil {
		panic(err)
	}
	for _, name := range names {
		nicknameMap[name] = name
	}
}

//捕获用户
func getMember(msg string) string {
	compile, err := regexp.Compile("tt(.+)tt")
	if err != nil {
		return ""
	}
	stringSubmatch := compile.FindAllStringSubmatch(msg, 1)
	if len(stringSubmatch) == 1 && len(stringSubmatch[0]) == 2 {
		return (stringSubmatch[0][1])
	}
	return ""
}
