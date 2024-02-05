package utils

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
	"text/template"
	"time"

	"github.com/spf13/viper"
)

var waitGroup sync.WaitGroup

func GetWaitGroup() *sync.WaitGroup {
	return &waitGroup
}

// PathExists check whether a path exists or not.
func PathExists(p string) bool {
	_, err := os.Stat(p)
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		return false
	}
	return false
}

// CreateRandomId create a random id with a given number N to add random numbers to the backend of strings.
func CreateRandomId(N int) string {
	rand.NewSource(time.Now().UnixNano())
	var letters = []rune("0123456789")
	b := make([]rune, N)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return time.Now().Format("200601021504") + string(b)
}

func CreateFileUploadDir(targetPath string) string {
	current := time.Now()
	dir := fmt.Sprintf("%d/%d/%d", current.Year(), current.Month(), current.Day())
	savePath, _ := os.Getwd()
	savePath = filepath.Join(filepath.Join(savePath, viper.GetString("blog.Dir")), targetPath, dir)
	err := os.MkdirAll(savePath, os.ModePerm)
	if err != nil {
		return ""
	}
	return savePath
}

func CreateFileName(suffix string) string {
	return fmt.Sprintf("%s.%s", CreateRandomId(4), suffix)
}

func TransferToDay(time time.Time) string {
	return fmt.Sprintf("%d-%02d-%02d", time.Year(), time.Month(), time.Day())
}

// GetCurrentDay get formatted current datetime for example: 2023-04-20
func GetCurrentDay() string {
	current := time.Now()
	return fmt.Sprintf("%d-%02d-%02d", current.Year(), current.Month(), current.Day())
}

// GenerateBlog creates the blog with template
func GenerateBlog(data any, templatePath string, targetPath string) (err error) {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	templatePath = filepath.Join(wd, templatePath)
	tmpl, err := template.ParseFiles(templatePath)
	if err != nil {
		return
	}
	file, err := os.OpenFile(targetPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
	defer file.Close()
	if err != nil {
		return
	}
	err = tmpl.Execute(file, data)
	if err != nil {
		return
	}
	return nil
}

// CoverBlog cover the blog with template
func CoverBlog(data any, templatePath string, targetPath string) (err error) {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	templatePath = filepath.Join(wd, templatePath)
	tmpl, err := template.ParseFiles(templatePath)
	if err != nil {
		return
	}
	file, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_TRUNC, 0666)
	defer file.Close()
	if err != nil {
		return
	}
	err = tmpl.Execute(file, data)
	if err != nil {
		return
	}
	return nil
}

// ParseTime
// @Author yizhigopher
// @Description 将1d20h30m10s字符串转化为具体的秒数
func ParseTime(timeStr string) int {
	expect := []time.Duration{24 * time.Hour, time.Hour, time.Minute, time.Second}
	re := regexp.MustCompile(`(?:([0-9]+)d)?(?:([0-9]+)h)?(?:([0-9]+)m)?(?:([0-9]+)s)?`)
	matches := re.FindStringSubmatch(timeStr)
	duration := time.Duration(0)
	for i, d := range matches {
		if i == 0 {
			continue
		}
		temp, _ := strconv.Atoi(d)
		duration += time.Duration(temp) * expect[i-1]
	}
	return int(duration.Seconds())
}
