package main

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/mysql"
	"github.com/robfig/cron"
)

var db *gorm.DB

// User 用户表
type User struct {
	gorm.Model
	UserName       string `gorm:"size:255"` // string默认长度为255, 使用这种tag重设。
	PhoneNum       string `gorm:"type:char(11)"`
	Password       string `json:"-"`
	Category       int
	Score          UserScore
	BillingAddress Address
}

// Loginlog 登录记录
type Loginlog struct {
	UserID uint
	Time   time.Time
	IP     string
}

// UserScore 用户积分表
type UserScore struct {
	UserID uint `gorm:"index"json:"-"`
	Score  uint64
	Energy uint64
	Other  uint64
}

// Scorelog 积分记录
type Scorelog struct {
	UserID   uint
	TrashID  string `gorm:"type:char(50)"`
	Time     time.Time
	AddScore uint64
}

// Address 用户住址表
type Address struct {
	UserID      int
	HomeAddress string `gorm:"not null;unique"` // 设置字段为非空并唯一
	WorkAddress string `gorm:"type:varchar(100);unique"`
}

// Notice 消息表
type Notice struct {
	gorm.Model
	UserID     uint64 `json:"-"`
	Title      string
	Content    string
	PictureURL string
}

// GarbageType 不同垃圾对应的分值
type GarbageType struct {
	gorm.Model
	Garbage string
	Score   uint8
}

// CheckStuats 状态检查
type CheckStuats struct {
	Stauts  uint32
	Message string
}

// Trash 垃圾桶位置信息
type Trash struct {
	ID        string
	Latitude  float64 `json:",omitempty"`
	Longitude float64 `json:",omitempty"`
	CoordType uint64  `json:"-"`
	Status    string
	IsFull    uint64         `json:"is_full"`
	New       GarbageInTrash `json:",omitempty"`
}

// GarbageInTrash 桶中的垃圾信息
type GarbageInTrash struct {
	gorm.Model
	TrashID string `json:"-"`
	Garbage string
}

// PHONENUMLENGTH 电话号码合法性匹配
var PHONENUMLENGTH = 11

// BAIDUMAPAPI 百度地图api url
var BAIDUMAPAPI = "api.map.baidu.com"

// POI 链接
var POI = "/geosearch/v3/nearby"

// AK 密钥
var AK = ""

// GEOTABLEID 地图poiid
var GEOTABLEID = ""

const (
	// ErrPhoneIsReg 手机号被注册
	ErrPhoneIsReg = 1000 + iota
	// ErrPhoneLengthWrong 手机号长度不对
	ErrPhoneLengthWrong
	// ErrIDisExist 用户ID不存在
	ErrIDisExist
	// ErrAuthFailed 认证失败
	ErrAuthFailed
	// ErrCateGoryWorry Category错误
	ErrCateGoryWorry
)

// 注册
func register(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	log.Println("注册" + r.FormValue("phonenum"))
	if r.Method == http.MethodPost {
		var user User
		var userscore UserScore
		var count int

		if len(r.FormValue("phonenum")) != 11 {
			log.Println(r.FormValue("phonenum") + "长度不正确")
			w.WriteHeader(ErrPhoneLengthWrong)
			return
		}
		db.Where("phone_num = ?", r.FormValue("phonenum")).Find(&User{}).Count(&count)
		if count == 0 {
			var loginlog Loginlog
			var err error
			user.UserName = r.FormValue("username")
			user.PhoneNum = r.FormValue("phonenum")
			user.Password = r.FormValue("password")
			user.Category, err = strconv.Atoi(r.FormValue("category"))
			if err != nil {
				w.WriteHeader(ErrCateGoryWorry)
				return
			}
			db.Create(&user)
			fmt.Println(user)
			var newuser User
			db.Where("phone_num = ?", r.FormValue("phonenum")).First(&newuser)
			fmt.Println(newuser.ID)
			userscore.UserID = newuser.ID
			loginlog.Time = time.Now()
			loginlog.UserID = newuser.ID
			loginlog.IP = r.RemoteAddr
			db.Create(&loginlog)
			db.Create(&userscore)

			var userscore UserScore
			db.Model(&user).Related(&userscore)
			user.Score = userscore
			js, err := json.Marshal(user)
			if err != nil {
				return
			}
			w.Header().Set("content-type", "application/json")
			fmt.Fprintf(w, string(js))
			log.Println("注册成功")
		} else {
			// 手机号被注册
			w.WriteHeader(ErrPhoneIsReg)
			log.Println(r.FormValue("phonenum") + "已经被注册")
		}
	}
}

// 获取积分
func getScore(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	log.Println("获取积分" + r.FormValue("phonenum"))
	if isIDRight(r.FormValue("id"), r.FormValue("password")) {
		var userscore UserScore
		db.Where("user_id = ?", r.FormValue("id")).First(&userscore)
		js, err := json.Marshal(userscore)
		if err != nil {
			return
		}
		w.Header().Set("content-type", "application/json")
		fmt.Fprintf(w, string(js))
	}
}

// 登录
func login(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	log.Println("登录" + r.FormValue("id"))
	if r.Method == http.MethodPost {
		var user User
		var loginlog Loginlog
		db.Where("phone_num = ?", r.FormValue("phonenum")).First(&user)
		if isPassRight(r.FormValue("phonenum"), r.FormValue("password")) {
			// 密码正确
			loginlog.Time = time.Now()
			loginlog.UserID = user.ID
			loginlog.IP = r.RemoteAddr
			db.Create(&loginlog)

			var userscore UserScore
			db.Model(&user).Related(&userscore)
			user.Score = userscore
			js, err := json.Marshal(user)
			if err != nil {
				return
			}
			w.Header().Set("content-type", "application/json")
			fmt.Fprintf(w, string(js))
		} else {
			// 密码错误
			w.WriteHeader(ErrAuthFailed)
			log.Println("密码错误")
		}
	}
}

// 获取用户基础信息
func getUserBasecData(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	log.Println("获取基础信息" + r.FormValue("id"))
	if r.Method == http.MethodPost {
		if isIDRight(r.FormValue("id"), r.FormValue("password")) {
			var userscore UserScore
			var user User
			db.Where("id = ?", r.FormValue("id")).First(&user)
			db.Model(&user).Related(&userscore)
			user.Score = userscore
			js, err := json.Marshal(user)
			if err != nil {
				return
			}
			w.Header().Set("content-type", "application/json")
			fmt.Fprintf(w, string(js))
		} else {
			w.WriteHeader(http.StatusBadRequest)
		}
	}
}

// 获取垃圾桶位置信息
func getTrashCan(w http.ResponseWriter, r *http.Request) {
	log.Println("获取垃圾桶位置" + "请求位置：" + r.FormValue("location") + "需要距离：" + r.FormValue("radius"))
	fmt.Println(r.FormValue("location"))
	url := "http://" + BAIDUMAPAPI + POI + "?" + "ak=" + AK + "&geotable_id=" + GEOTABLEID + "&location=" + r.FormValue("location") + "&radius=" + r.FormValue("radius") + "&coord_type=" + r.FormValue("coord_type")
	req, _ := http.NewRequest(r.Method, url, nil)

	res, _ := http.DefaultClient.Do(req)

	defer res.Body.Close()
	body, _ := ioutil.ReadAll(res.Body)

	fmt.Println(string(body))
	w.Header().Set("content-type", "application/json")
	fmt.Fprintf(w, string(body))
}

// 增加积分
func addScore(w http.ResponseWriter, r *http.Request) {
	log.Println("增加积分")
	r.ParseForm()
	if r.Method == http.MethodPost {
		if isIDRight(r.FormValue("id"), r.FormValue("password")) {
			var userscore UserScore
			var scorelog Scorelog
			var err error
			var garbageintrash GarbageInTrash
			var garbagetype GarbageType
			db.Where("user_id = ?", r.FormValue("id")).First(&userscore)
			// db.Where("trash_id = ?", r.FormValue("trashid")).First(&garbageintrash)
			db.Where("trash_id = ? AND MINUTE( timediff( now(), updated_at) ) <= 1", r.FormValue("trashid")).First(&garbageintrash)
			if garbageintrash.DeletedAt != nil || garbageintrash.TrashID == "" {
				fmt.Fprintf(w, "0")
				return
			}
			db.Where("garbage = ?", garbageintrash.Garbage).First(&garbagetype)
			db.Delete(&garbageintrash)
			db.Model(&userscore).Update("score", userscore.Score+uint64(garbagetype.Score))
			scorelog.Time = time.Now()
			scorelog.UserID = userscore.UserID
			scorelog.TrashID = r.FormValue("trashid")
			if err != nil {
				w.WriteHeader(ErrCateGoryWorry)
				return
			}
			scorelog.AddScore = uint64(garbagetype.Score)
			db.Create(&scorelog)
			fmt.Fprintf(w, "%d", garbagetype.Score)
		} else {
			// 密码错误
			w.WriteHeader(ErrAuthFailed)
			log.Println("密码错误")
		}
	}
}

// 得到最新公告
func getLastNotice(w http.ResponseWriter, r *http.Request) {
	var notice Notice
	db.Last(&notice)
	js, err := json.Marshal(notice)
	if err != nil {
		return
	}
	w.Header().Set("content-type", "application/json")
	fmt.Fprintf(w, string(js))
}

// 得到所有公告
func getAllNotice(w http.ResponseWriter, r *http.Request) {
	var notice []Notice
	db.Find(&notice)
	js, err := json.Marshal(notice)
	if err != nil {
		return
	}
	w.Header().Set("content-type", "application/json")
	fmt.Fprintf(w, string(js))
}

// 验证密码是否正确
func isPassRight(PhoneNum string, Password string) bool {
	var user User
	db.Where("phone_num = ?", PhoneNum).First(&user)
	if Password == user.Password {
		return true
	}
	return false

}

// 使用ID验证密码是否正确
func isIDRight(id string, Password string) bool {
	var user User
	db.Where("id = ?", id).First(&user)
	if Password == user.Password {
		return true
	}
	return false
}

func checkGarbage() {
	log.Println("检查数据")
	db.Where("MINUTE( timediff( now(), created_at) ) >= 1").Delete(&GarbageInTrash{})
}

// tcp获取垃圾桶位置信息
func handleClient(conn net.Conn) {
	defer conn.Close()
	buf := make([]byte, 1024)
	length, _ := conn.Read(buf)
	var trash Trash

	log.Println(string(buf[0:length]))

	if err := json.Unmarshal(buf[0:length], &trash); err != nil {
		log.Printf("JSON unmarshaling failed: %s", err)
		conn.Write([]byte("error"))
		return
	}

	form := url.Values{}
	form.Add("latitude", strconv.FormatFloat(trash.Latitude, 'g', -1, 64))
	form.Add("longitude", strconv.FormatFloat(trash.Longitude, 'g', -1, 64))
	form.Add("coord_type", "1")
	form.Add("geotable_id", GEOTABLEID)
	form.Add("ak", AK)

	var resp *http.Response
	var err error
	switch trash.Status {
	case "add":
		log.Println("添加垃圾箱")
		resp, err = http.PostForm("http://api.map.baidu.com/geodata/v3/poi/create", form)

		log.Println(form.Encode())

		if err != nil {
			// handle error
			log.Fatalf("post unmarshaling failed: %s", err)
		}

		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			// handle error
		}

		fmt.Println(string(body))

		conn.Write(body) // don't care about return value
		// we're finished with this client
		break
	case "update":
		log.Println("更新垃圾箱信息")
		form.Add("is_full", strconv.FormatUint(trash.IsFull, 10))
		form.Add("id", trash.ID)
		resp, err = http.PostForm("http://api.map.baidu.com/geodata/v3/poi/update", form)

		if err != nil {
			// handle error
			log.Fatalf("post unmarshaling failed: %s", err)
		}

		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			// handle error
		}

		log.Println(string(body))

		conn.Write(body) // don't care about return value
		// we're finished with this client
		break
	case "havetrash":
		log.Println("有垃圾投入")
		garbageintrash := trash.New
		h := md5.New()
		io.WriteString(h, trash.ID)
		garbageintrash.TrashID = fmt.Sprintf("%x", md5.Sum(h.Sum(nil)[4:14]))
		db.Create(&garbageintrash)
		conn.Write([]byte("200"))
		break
	default:
		return
	}
}

func main() {
	var err error
	db, err = gorm.Open("mysql", "oak:root@/test?charset=utf8&parseTime=True&loc=Local")
	if err != nil {
		panic("连接数据库失败")
	}
	defer db.Close()

	db.AutoMigrate(&User{})
	db.AutoMigrate(&Loginlog{})
	db.AutoMigrate(&UserScore{})
	db.AutoMigrate(&Scorelog{})
	db.AutoMigrate(&Address{})
	db.AutoMigrate(&Notice{})
	db.AutoMigrate(&GarbageType{})
	db.AutoMigrate(&GarbageInTrash{})

	// 登录
	http.HandleFunc("/login", login)
	// 注册
	http.HandleFunc("/register", register)
	// 获取积分
	http.HandleFunc("/getscore", getScore)
	// 获取用户基础信息
	http.HandleFunc("/getbasic", getUserBasecData)
	// 获取垃圾桶位置信息
	http.HandleFunc("/gettrashcan", getTrashCan)
	// 增加积分
	http.HandleFunc("/addscore", addScore)
	// 得到最新公告
	http.HandleFunc("/getlastnotice", getLastNotice)
	// 得到所有公告
	http.HandleFunc("/getallnotice", getAllNotice)
	go http.ListenAndServe(":6060", nil)
	// err = http.ListenAndServe(":6060", nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}

	c := cron.New()
	specCheckGarbageInTrash := "0 1-2 * * * *"

	c.AddFunc(specCheckGarbageInTrash, checkGarbage)
	c.Start()

	service := ":7070"
	tcpAddr, err := net.ResolveTCPAddr("tcp4", service)
	checkError(err)
	listener, err := net.ListenTCP("tcp", tcpAddr)
	checkError(err)
	for {
		conn, err := listener.Accept()
		if err != nil {
			continue
		}
		go handleClient(conn)
	}
}

func checkError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal error: %s", err.Error())
		os.Exit(1)
	}
}
