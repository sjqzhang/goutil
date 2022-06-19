package goutil

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/deckarep/golang-set"
	_ "github.com/mattn/go-sqlite3"
	"io"
	"io/ioutil"
	"log"
	random "math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type CommonMap struct {
	sync.RWMutex
	m map[string]interface{}
}

type Tuple struct {
	Key string
	Val interface{}
}

type Common struct {
}

func NewCommonMap(size int) *CommonMap {
	if size > 0 {
		return &CommonMap{m: make(map[string]interface{}, size)}
	} else {
		return &CommonMap{m: make(map[string]interface{})}
	}
}
func (s *CommonMap) GetValue(k string) (interface{}, bool) {
	s.RLock()
	defer s.RUnlock()
	v, ok := s.m[k]
	return v, ok
}
func (s *CommonMap) Put(k string, v interface{}) {
	s.Lock()
	defer s.Unlock()
	s.m[k] = v
}
func (s *CommonMap) Iter() <-chan Tuple { // reduce memory
	ch := make(chan Tuple)
	go func() {
		s.RLock()
		for k, v := range s.m {
			ch <- Tuple{Key: k, Val: v}
		}
		close(ch)
		s.RUnlock()
	}()
	return ch
}
func (s *CommonMap) LockKey(k string) {
	s.Lock()
	if v, ok := s.m[k]; ok {
		s.m[k+"_lock_"] = true
		s.Unlock()
		switch v.(type) {
		case *sync.Mutex:
			v.(*sync.Mutex).Lock()
		default:
			log.Print(fmt.Sprintf("LockKey %s", k))
		}
	} else {
		s.m[k] = &sync.Mutex{}
		v = s.m[k]
		s.m[k+"_lock_"] = true
		v.(*sync.Mutex).Lock()
		s.Unlock()
	}
}
func (s *CommonMap) UnLockKey(k string) {
	s.Lock()
	if v, ok := s.m[k]; ok {
		switch v.(type) {
		case *sync.Mutex:
			v.(*sync.Mutex).Unlock()
		default:
			log.Print(fmt.Sprintf("UnLockKey %s", k))
		}
		delete(s.m, k+"_lock_") // memory leak
		delete(s.m, k)          // memory leak
	}
	s.Unlock()
}
func (s *CommonMap) IsLock(k string) bool {
	s.Lock()
	if v, ok := s.m[k+"_lock_"]; ok {
		s.Unlock()
		return v.(bool)
	}
	s.Unlock()
	return false
}
func (s *CommonMap) Keys() []string {
	s.Lock()
	keys := make([]string, len(s.m))
	defer s.Unlock()
	for k, _ := range s.m {
		keys = append(keys, k)
	}
	return keys
}
func (s *CommonMap) Clear() {
	s.Lock()
	defer s.Unlock()
	s.m = make(map[string]interface{})
}
func (s *CommonMap) Remove(key string) {
	s.Lock()
	defer s.Unlock()
	if _, ok := s.m[key]; ok {
		delete(s.m, key)
	}
}
func (s *CommonMap) AddUniq(key string) {
	s.Lock()
	defer s.Unlock()
	if _, ok := s.m[key]; !ok {
		s.m[key] = nil
	}
}
func (s *CommonMap) AddCount(key string, count int) {
	s.Lock()
	defer s.Unlock()
	if _v, ok := s.m[key]; ok {
		v := _v.(int)
		v = v + count
		s.m[key] = v
	} else {
		s.m[key] = 1
	}
}
func (s *CommonMap) AddCountInt64(key string, count int64) {
	s.Lock()
	defer s.Unlock()
	if _v, ok := s.m[key]; ok {
		v := _v.(int64)
		v = v + count
		s.m[key] = v
	} else {
		s.m[key] = count
	}
}
func (s *CommonMap) Add(key string) {
	s.Lock()
	defer s.Unlock()
	if _v, ok := s.m[key]; ok {
		v := _v.(int)
		v = v + 1
		s.m[key] = v
	} else {
		s.m[key] = 1
	}
}
func (s *CommonMap) Zero() {
	s.Lock()
	defer s.Unlock()
	for k := range s.m {
		s.m[k] = 0
	}
}
func (s *CommonMap) Contains(i ...interface{}) bool {
	s.Lock()
	defer s.Unlock()
	for _, val := range i {
		if _, ok := s.m[val.(string)]; !ok {
			return false
		}
	}
	return true
}
func (s *CommonMap) Get() map[string]interface{} {
	s.Lock()
	defer s.Unlock()
	m := make(map[string]interface{})
	for k, v := range s.m {
		m[k] = v
	}
	return m
}

func (this *Common) GetUUID() string {
	b := make([]byte, 48)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return ""
	}
	id := this.MD5(base64.URLEncoding.EncodeToString(b))
	return fmt.Sprintf("%s-%s-%s-%s-%s", id[0:8], id[8:12], id[12:16], id[16:20], id[20:])
}
func (this *Common) CopyFile(src, dst string) (int64, error) {
	sourceFileStat, err := os.Stat(src)
	if err != nil {
		return 0, err
	}
	if !sourceFileStat.Mode().IsRegular() {
		return 0, fmt.Errorf("%s is not a regular file", src)
	}
	source, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer source.Close()
	destination, err := os.Create(dst)
	if err != nil {
		return 0, err
	}
	defer destination.Close()
	nBytes, err := io.Copy(destination, source)
	return nBytes, err
}
func (this *Common) RandInt(min, max int) int {
	return func(min, max int) int {
		r := random.New(random.NewSource(time.Now().UnixNano()))
		if min >= max {
			return max
		}
		return r.Intn(max-min) + min
	}(min, max)
}
func (this *Common) GetToDay() string {
	return time.Now().Format("20060102")
}
func (this *Common) UrlEncode(v interface{}) string {
	switch v.(type) {
	case string:
		m := make(map[string]string)
		m["name"] = v.(string)
		return strings.Replace(this.UrlEncodeFromMap(m), "name=", "", 1)
	case map[string]string:
		return this.UrlEncodeFromMap(v.(map[string]string))
	default:
		return fmt.Sprintf("%v", v)
	}
}
func (this *Common) UrlEncodeFromMap(m map[string]string) string {
	vv := url.Values{}
	for k, v := range m {
		vv.Add(k, v)
	}
	return vv.Encode()
}
func (this *Common) UrlDecodeToMap(body string) (map[string]string, error) {
	var (
		err error
		m   map[string]string
		v   url.Values
	)
	m = make(map[string]string)
	if v, err = url.ParseQuery(body); err != nil {
		return m, err
	}
	for _k, _v := range v {
		if len(_v) > 0 {
			m[_k] = _v[0]
		}
	}
	return m, nil
}
func (this *Common) GetDayFromTimeStamp(timeStamp int64) string {
	return time.Unix(timeStamp, 0).Format("20060102")
}
func (this *Common) StrToMapSet(str string, sep string) mapset.Set {
	result := mapset.NewSet()
	for _, v := range strings.Split(str, sep) {
		result.Add(v)
	}
	return result
}
func (this *Common) MapSetToStr(set mapset.Set, sep string) string {
	var (
		ret []string
	)
	for v := range set.Iter() {
		ret = append(ret, v.(string))
	}
	return strings.Join(ret, sep)
}
func (this *Common) GetPulicIP() string {
	var (
		err  error
		conn net.Conn
	)
	if conn, err = net.Dial("udp", "8.8.8.8:80"); err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()
	localAddr := conn.LocalAddr().String()
	idx := strings.LastIndex(localAddr, ":")
	return localAddr[0:idx]
}
func (this *Common) MD5(str string) string {
	md := md5.New()
	md.Write([]byte(str))
	return fmt.Sprintf("%x", md.Sum(nil))
}
func (this *Common) GetFileMd5(file *os.File) string {
	file.Seek(0, 0)
	md5h := md5.New()
	io.Copy(md5h, file)
	sum := fmt.Sprintf("%x", md5h.Sum(nil))
	return sum
}
func (this *Common) GetFileSum(file *os.File, alg string) string {
	alg = strings.ToLower(alg)
	if alg == "sha1" {
		return this.GetFileSha1Sum(file)
	} else {
		return this.GetFileMd5(file)
	}
}
func (this *Common) GetFileSumByName(filepath string, alg string) (string, error) {
	var (
		err  error
		file *os.File
	)
	file, err = os.Open(filepath)
	if err != nil {
		return "", err
	}
	defer file.Close()
	alg = strings.ToLower(alg)
	if alg == "sha1" {
		return this.GetFileSha1Sum(file), nil
	} else {
		return this.GetFileMd5(file), nil
	}
}
func (this *Common) GetFileSha1Sum(file *os.File) string {
	file.Seek(0, 0)
	md5h := sha1.New()
	io.Copy(md5h, file)
	sum := fmt.Sprintf("%x", md5h.Sum(nil))
	return sum
}
func (this *Common) WriteFileByOffSet(filepath string, offset int64, data []byte) error {
	var (
		err   error
		file  *os.File
		count int
	)
	file, err = os.OpenFile(filepath, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		return err
	}
	defer file.Close()
	count, err = file.WriteAt(data, offset)
	if err != nil {
		return err
	}
	if count != len(data) {
		return errors.New(fmt.Sprintf("write %s error", filepath))
	}
	return nil
}
func (this *Common) ReadFileByOffSet(filepath string, offset int64, length int) ([]byte, error) {
	var (
		err    error
		file   *os.File
		result []byte
		count  int
	)
	file, err = os.Open(filepath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	result = make([]byte, length)
	count, err = file.ReadAt(result, offset)
	if err != nil {
		return nil, err
	}
	if count != length {
		return nil, errors.New("read error")
	}
	return result, nil
}
func (this *Common) Contains(obj interface{}, arrayobj interface{}) bool {
	targetValue := reflect.ValueOf(arrayobj)
	switch reflect.TypeOf(arrayobj).Kind() {
	case reflect.Slice, reflect.Array:
		for i := 0; i < targetValue.Len(); i++ {
			if targetValue.Index(i).Interface() == obj {
				return true
			}
		}
	case reflect.Map:
		if targetValue.MapIndex(reflect.ValueOf(obj)).IsValid() {
			return true
		}
	}
	return false
}
func (this *Common) FileExists(fileName string) bool {
	_, err := os.Stat(fileName)
	return err == nil
}
func (this *Common) WriteFile(path string, data string) bool {
	if err := ioutil.WriteFile(path, []byte(data), 0775); err == nil {
		return true
	} else {
		return false
	}
}
func (this *Common) WriteBinFile(path string, data []byte) bool {
	if err := ioutil.WriteFile(path, data, 0775); err == nil {
		return true
	} else {
		return false
	}
}
func (this *Common) IsExist(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil || os.IsExist(err)
}
func (this *Common) Match(matcher string, content string) []string {
	var result []string
	if reg, err := regexp.Compile(matcher); err == nil {
		result = reg.FindAllString(content, -1)
	}
	return result
}
func (this *Common) ReadBinFile(path string) ([]byte, error) {
	if this.IsExist(path) {
		fi, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer fi.Close()
		return ioutil.ReadAll(fi)
	} else {
		return nil, errors.New("not found")
	}
}

func (this *Common) JsonDecode(jsonstr string) interface{} {

	var v interface{}
	err := json.Unmarshal([]byte(jsonstr), &v)
	if err != nil {
		return nil

	} else {
		return v
	}

}

func (this *Common) Color(m string, c string) string {
	color := func(m string, c string) string {
		colorMap := make(map[string]string)
		if c == "" {
			c = "green"
		}
		black := fmt.Sprintf("\033[30m%s\033[0m", m)
		red := fmt.Sprintf("\033[31m%s\033[0m", m)
		green := fmt.Sprintf("\033[32m%s\033[0m", m)
		yello := fmt.Sprintf("\033[33m%s\033[0m", m)
		blue := fmt.Sprintf("\033[34m%s\033[0m", m)
		purple := fmt.Sprintf("\033[35m%s\033[0m", m)
		white := fmt.Sprintf("\033[37m%s\033[0m", m)
		glint := fmt.Sprintf("\033[5;31m%s\033[0m", m)
		colorMap["black"] = black
		colorMap["red"] = red
		colorMap["green"] = green
		colorMap["yello"] = yello
		colorMap["yellow"] = yello
		colorMap["blue"] = blue
		colorMap["purple"] = purple
		colorMap["white"] = white
		colorMap["glint"] = glint
		if v, ok := colorMap[c]; ok {
			return v
		} else {
			return colorMap["green"]
		}
	}
	return color(m, c)
}

type OptionJF struct {
	m map[string]string
}

func (this *OptionJF) Where(condition string) *OptionJF {
	this.m["w"] = condition
	return this
}

func (this *OptionJF) Columns(columns string) *OptionJF {
	this.m["c"] = columns
	return this
}

func (this *OptionJF) Limit(start int, limit int) *OptionJF {
	this.m["limit"] = fmt.Sprintf(" %v offset %v", limit, start)
	return this
}

func NewJfOption() *OptionJF {
	opt := OptionJF{m: make(map[string]string)}
	opt.m["c"] = "*"
	opt.m["w"] = "1=1"
	opt.m["limit"] = " 1000 offset 0"
	return &opt
}

type logger struct {
	tag string
	log *log.Logger
}

func NewLogger(tag string) *logger {
	return &logger{tag: tag, log: log.New(os.Stdout, fmt.Sprintf("[%v] ", tag), log.LstdFlags)}
}
func (l *logger) SetTag(tag string) {
	l.tag = tag
}
func (l *logger) Log(msg interface{}) {
	l.log.Println("\u001B[32m" + fmt.Sprintf("%v", msg) + "\u001B[0m")
}
func (l *logger) Warn(msg interface{}) {
	l.log.Println("\u001B[33m" + fmt.Sprintf("%v", msg) + "\u001B[0m")
}
func (l *logger) Error(msg interface{}) {

	l.log.Println("\u001B[31m" + fmt.Sprintf("%v", msg) + "\u001B[0m")
}
func (l *logger) Panic(msg interface{}) {
	panic("\u001B[31m" + fmt.Sprintf("%v", msg) + "\u001B[0m")
}

var Log *logger = NewLogger("goutil")

func (this *Common) Jf(data interface{}, opt *OptionJF) []map[string]interface{} {

	//data, _ := this.StdinJson(module, action)

	data = this.JsonDecode(this.JsonEncodePretty(data))

	var records []map[string]interface{}

	c := "*"
	w := "1=1"
	s := "select %s from data where 1=1 and %s %s"

	limit := ""
	argv := opt.m
	if v, ok := argv["c"]; ok {
		c = v
	}

	if v, ok := argv["w"]; ok {
		w = v
	}

	if v, ok := argv["limit"]; ok {
		limit = " limit " + v
	}

	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		Log.Error(err)
		//log.Flush()
		return records
	}

	Push := func(db *sql.DB, records []interface{}) error {
		hashKeys := map[string]struct{}{}

		keyword := []string{"ALTER",
			"CLOSE",
			"COMMIT",
			"CREATE",
			"DECLARE",
			"DELETE",
			"DENY",
			"DESCRIBE",
			"DOMAIN",
			"DROP",
			"EXECUTE",
			"EXPLAN",
			"FETCH",
			"GRANT",
			"INDEX",
			"INSERT",
			"OPEN",
			"PREPARE",
			"PROCEDURE",
			"REVOKE",
			"ROLLBACK",
			"SCHEMA",
			"SELECT",
			"SET",
			"SQL",
			"TABLE",
			"TRANSACTION",
			"TRIGGER",
			"UPDATE",
			"VIEW",
			"GROUP"}

		_ = keyword

		//for _, record := range records {
		//	switch record.(type) {
		//	case map[string]interface{}:
		//		for key, _ := range record.(map[string]interface{}) {
		//			key2 := key
		//			if this.util.Contains(strings.ToUpper(key), keyword) {
		//				key2 = "_" + key
		//				record.(map[string]interface{})[key2] = record.(map[string]interface{})[key]
		//				delete(record.(map[string]interface{}), key)
		//			}
		//			hashKeys[key2] = struct{}{}
		//		}
		//	}
		//}

		for _, record := range records {
			switch record.(type) {
			case map[string]interface{}:
				for key, _ := range record.(map[string]interface{}) {
					if strings.HasPrefix(key, "`") {
						continue
					}
					key2 := fmt.Sprintf("`%s`", key)
					record.(map[string]interface{})[key2] = record.(map[string]interface{})[key]
					delete(record.(map[string]interface{}), key)
					hashKeys[key2] = struct{}{}
				}
			}
		}

		keys := []string{}

		for key, _ := range hashKeys {
			keys = append(keys, key)
		}
		//		db.Exec("DROP TABLE data")
		query := "CREATE TABLE data (" + strings.Join(keys, ",") + ")"
		if _, err := db.Exec(query); err != nil {
			Log.Error(query)
			//log.Flush()
			return err
		}

		for _, record := range records {
			recordKeys := []string{}
			recordValues := []string{}
			recordArgs := []interface{}{}

			switch record.(type) {
			case map[string]interface{}:

				for key, value := range record.(map[string]interface{}) {
					recordKeys = append(recordKeys, key)
					recordValues = append(recordValues, "?")
					recordArgs = append(recordArgs, value)
				}

			}

			query := "INSERT INTO data (" + strings.Join(recordKeys, ",") +
				") VALUES (" + strings.Join(recordValues, ", ") + ")"

			statement, err := db.Prepare(query)
			if err != nil {
				Log.Error(err)
				//Log.Error(err, "can't prepare query: %s", query, recordKeys, recordArgs, recordValues)
				//log.Flush()
				continue

			}

			_, err = statement.Exec(recordArgs...)
			if err != nil {
				Log.Error(err)
				//Log.Error(err, "can't insert record", )
			}
			statement.Close()
		}

		return nil
	}

	switch data.(type) {
	case []interface{}:
		err = Push(db, data.([]interface{}))
		if err != err {
			fmt.Println(err)
			return records
		}
	default:
		fmt.Println(reflect.TypeOf(data).Kind().String())
		msg := "(error) just support list"
		fmt.Println(msg)
		Log.Error(msg)
		return records

	}

	defer db.Close()
	if err == nil {
		db.SetMaxOpenConns(1)
	} else {
		Log.Error(err.Error())
	}

	s = fmt.Sprintf(s, c, w, limit)

	rows, err := db.Query(s)

	if err != nil {
		Log.Error(err)
		Log.Error(s)
		//log.Flush()
		fmt.Println(err)
		return records
	}
	defer rows.Close()

	records = []map[string]interface{}{}
	for rows.Next() {
		record := map[string]interface{}{}

		columns, err := rows.Columns()
		if err != nil {
			Log.Error(err)
			//Log.Error(	err, "unable to obtain rows columns",
			//)
			continue
		}

		pointers := []interface{}{}
		for _, column := range columns {
			var value interface{}
			pointers = append(pointers, &value)
			record[column] = &value
		}

		err = rows.Scan(pointers...)
		if err != nil {
			Log.Error(fmt.Sprintf("%v %v", err, "can't read result records"))
			continue
		}

		for key, value := range record {
			indirect := *value.(*interface{})
			if value, ok := indirect.([]byte); ok {
				record[key] = string(value)
			} else {
				record[key] = indirect
			}
		}

		records = append(records, record)
	}

	return records

	//fmt.Println(util.JsonEncodePretty(records))

}

func (this *Common) Replace(s string, o string, n string) string {
	reg := regexp.MustCompile(o)
	s = reg.ReplaceAllString(s, n)
	return s
}

func (this *Common) Jq(data interface{}, key string) interface{} {
	if v, ok := data.(string); ok {
		data = this.JsonDecode(v)
	} else {
		data = this.JsonDecode(this.JsonEncodePretty(data))
	}
	//if v, ok := data.([]byte); ok {
	//	data = this.JsonDecode(string(v))
	//}
	//if _, ok := data.([]map[string]interface{}); ok {
	//	data = this.JsonDecode(this.JsonEncodePretty(data))
	//}

	var obj interface{}
	var ks []string
	if strings.Contains(key, ",") {
		ks = strings.Split(key, ",")
	} else {
		ks = strings.Split(key, ".")
	}
	obj = data

	ParseDict := func(obj interface{}, key string) interface{} {
		switch obj.(type) {
		case map[string]interface{}:
			if v, ok := obj.(map[string]interface{})[key]; ok {
				return v
			}
		}
		return nil

	}

	ParseList := func(obj interface{}, key string) interface{} {
		var ret []interface{}
		switch obj.(type) {
		case []interface{}:
			if ok, _ := regexp.MatchString("^\\d+$", key); ok {
				i, _ := strconv.Atoi(key)
				return obj.([]interface{})[i]
			}

			for _, v := range obj.([]interface{}) {
				switch v.(type) {
				case map[string]interface{}:
					if key == "*" {
						for _, vv := range v.(map[string]interface{}) {
							ret = append(ret, vv)
						}
					} else {
						if vv, ok := v.(map[string]interface{})[key]; ok {
							ret = append(ret, vv)
						}
					}
				case []interface{}:
					if key == "*" {
						for _, vv := range v.([]interface{}) {
							ret = append(ret, vv)
						}
					} else {
						ret = append(ret, v)
					}
				}
			}
		}
		return ret
	}
	if key != "" {
		for _, k := range ks {
			switch obj.(type) {
			case map[string]interface{}:
				obj = ParseDict(obj, k)
			case []interface{}:
				obj = ParseList(obj, k)
			}
		}
	}
	return obj
}

func (this *Common) RemoveEmptyDir(pathname string) {
	defer func() {
		if re := recover(); re != nil {
			buffer := debug.Stack()
			log.Print(string(buffer))
		}
	}()
	handlefunc := func(file_path string, f os.FileInfo, err error) error {
		if f.IsDir() {
			files, _ := ioutil.ReadDir(file_path)
			if len(files) == 0 && file_path != pathname {
				os.Remove(file_path)
			}
		}
		return nil
	}
	fi, _ := os.Stat(pathname)
	if fi.IsDir() {
		filepath.Walk(pathname, handlefunc)
	}
}
func (this *Common) JsonEncodePretty(o interface{}) string {
	resp := ""
	switch o.(type) {
	case map[string]interface{}:
		if data, err := json.Marshal(o); err == nil {
			resp = string(data)
		}
	case map[string]string:
		if data, err := json.Marshal(o); err == nil {
			resp = string(data)
		}
	case []interface{}:
		if data, err := json.Marshal(o); err == nil {
			resp = string(data)
		}
	case []string:
		if data, err := json.Marshal(o); err == nil {
			resp = string(data)
		}
	case string:
		resp = o.(string)
	default:
		if data, err := json.Marshal(o); err == nil {
			resp = string(data)
		}
	}
	var v interface{}
	if ok := json.Unmarshal([]byte(resp), &v); ok == nil {
		if buf, ok := json.MarshalIndent(v, "", "  "); ok == nil {
			resp = string(buf)
		}
	}
	resp = strings.Replace(resp, "\\u003c", "<", -1)
	resp = strings.Replace(resp, "\\u003e", ">", -1)
	resp = strings.Replace(resp, "\\u0026", "&", -1)
	return resp
}
func (this *Common) GetClientIp(r *http.Request) string {
	client_ip := ""
	headers := []string{"X_Forwarded_For", "X-Forwarded-For", "X-Real-Ip",
		"X_Real_Ip", "Remote_Addr", "Remote-Addr"}
	for _, v := range headers {
		if _v, ok := r.Header[v]; ok {
			if len(_v) > 0 {
				client_ip = _v[0]
				break
			}
		}
	}
	if client_ip == "" {
		clients := strings.Split(r.RemoteAddr, ":")
		client_ip = clients[0]
	}
	return client_ip
}
func (this *Common) Exec(cmd []string, timeout int) (string, int) {
	var out bytes.Buffer
	duration := time.Duration(timeout) * time.Second
	ctx, _ := context.WithTimeout(context.Background(), duration)
	var command *exec.Cmd
	command = exec.CommandContext(ctx, cmd[0], cmd[1:]...)
	command.Stdin = os.Stdin
	command.Stdout = &out
	command.Stderr = &out
	err := command.Run()
	if err != nil {
		log.Println(err, cmd)
		return err.Error(), -1
	}
	status := command.ProcessState.Sys().(syscall.WaitStatus).ExitStatus()
	return out.String(), status
}

func (this *Common) GetAllIpsV4() ([]string, error) {
	var (
		ips    []string
		allIps []string
		err    error
	)
	if allIps, err = this.GetAllIps(); err != nil {
		return ips, err
	}
	for _, ip := range allIps {
		i := this.Match("\\d+\\.\\d+\\.\\d+\\.\\d+", ip)
		if len(i) > 0 {
			ips = append(ips, i[0])
		}
	}
	return ips, nil
}

func (this *Common) GetAllIpsV6() ([]string, error) {
	var (
		ips    []string
		allIps []string
		err    error
	)
	if allIps, err = this.GetAllIps(); err != nil {
		return ips, err
	}
	for _, ip := range allIps {
		i := this.Match("[0-9a-z:]{15,}", ip)
		if len(i) > 0 {
			ips = append(ips, i[0])
		}
	}
	return ips, nil
}
func (this *Common) GetAllMacs() ([]string, error) {
	var (
		err        error
		interfaces []net.Interface
		macs       []string
	)
	if interfaces, err = net.Interfaces(); err != nil {
		return macs, err
	}
	for _, v := range interfaces {
		macs = append(macs, v.HardwareAddr.String())
	}
	return macs, nil
}

func (this *Common) GetAllIps() ([]string, error) {
	var (
		err        error
		interfaces []net.Interface
		ips        []string
		addrs      []net.Addr
	)
	if interfaces, err = net.Interfaces(); err != nil {
		return ips, err
	}
	for _, v := range interfaces {
		if addrs, err = v.Addrs(); err == nil {
			for _, addr := range addrs {
				ips = append(ips, addr.String())
			}
		}
	}
	return ips, nil
}
