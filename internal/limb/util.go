package limb

import (
	"compress/gzip"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"

	"github.com/duo/octopus-wechat-web/internal/common"

	"github.com/antchfx/xmlquery"
	"github.com/eatmoreapple/openwechat"
	"github.com/gabriel-vasile/mimetype"

	log "github.com/sirupsen/logrus"
)

var (
	httpClient = &http.Client{
		Transport: &http.Transport{
			ForceAttemptHTTP2:   true,
			MaxConnsPerHost:     0,
			MaxIdleConns:        0,
			MaxIdleConnsPerHost: 256,
		},
	}

	UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/87.0.4280.88 Safari/537.36 Edg/87.0.664.66"
)

func parseApp(b *Bot, content string, appType openwechat.AppMessageType) *common.AppData {
	doc, err := xmlquery.Parse(strings.NewReader(content))
	if err != nil {
		return nil
	}

	switch appType {
	case 1:
		titleNode := xmlquery.FindOne(doc, "/msg/appmsg/title")
		if titleNode == nil || len(titleNode.InnerText()) == 0 {
			return nil
		}
		return &common.AppData{
			Title:       "",
			Description: titleNode.InnerText(),
			Source:      "",
			URL:         "",
		}
	case 19: // forward
		titleNode := xmlquery.FindOne(doc, "/msg/appmsg/title")
		if titleNode == nil || len(titleNode.InnerText()) == 0 {
			return nil
		}
		var des string
		desNode := xmlquery.FindOne(doc, "/msg/appmsg/des")
		if desNode != nil {
			des = desNode.InnerText()
		}
		return &common.AppData{
			Title:       titleNode.InnerText(),
			Description: des,
			Source:      "",
			URL:         "",
		}
	case 51: // video
		titleNode := xmlquery.FindOne(doc, "/msg/appmsg/finderFeed/nickname")
		if titleNode == nil || len(titleNode.InnerText()) == 0 {
			return nil
		}
		var des string
		desNode := xmlquery.FindOne(doc, "/msg/appmsg/finderFeed/desc")
		if desNode != nil {
			des = desNode.InnerText()
		}
		var url string
		urlNode := xmlquery.FindOne(doc, "/msg/appmsg/finderFeed//fullCoverUrl")
		if urlNode != nil {
			url = urlNode.InnerText()
		}
		return &common.AppData{
			Title:       titleNode.InnerText(),
			Description: des,
			Source:      titleNode.InnerText(),
			URL:         url,
		}
	case 63: // live
		titleNode := xmlquery.FindOne(doc, "/msg/appmsg/finderLive/nickname")
		if titleNode == nil || len(titleNode.InnerText()) == 0 {
			return nil
		}
		var des string
		desNode := xmlquery.FindOne(doc, "/msg/appmsg/finderLive/desc")
		if desNode != nil {
			des = desNode.InnerText()
		}
		var url string
		urlNode := xmlquery.FindOne(doc, "/msg/appmsg/finderLive//coverUrl")
		if urlNode != nil {
			url = urlNode.InnerText()
		}
		return &common.AppData{
			Title:       titleNode.InnerText(),
			Description: des,
			Source:      titleNode.InnerText(),
			URL:         url,
		}
	default:
		titleNode := xmlquery.FindOne(doc, "/msg/appmsg/title")
		if titleNode == nil || len(titleNode.InnerText()) == 0 {
			return nil
		}
		var url string
		urlNode := xmlquery.FindOne(doc, "/msg/appmsg/url")
		if urlNode != nil {
			url = urlNode.InnerText()
		}
		var des string
		desNode := xmlquery.FindOne(doc, "/msg/appmsg/des")
		if desNode != nil {
			des = desNode.InnerText()
		}
		var source string
		if sourceNode := xmlquery.FindOne(doc, "/msg/appmsg/sourcedisplayname"); sourceNode != nil {
			source = sourceNode.InnerText()
		} else if sourceNode := xmlquery.FindOne(doc, "/msg/appinfo/appname"); sourceNode != nil {
			source = sourceNode.InnerText()
		}
		return &common.AppData{
			Title:       titleNode.InnerText(),
			Description: des,
			Source:      source,
			URL:         url,
		}
	}
}

func downloadSticker(b *Bot, content string) *common.BlobData {
	doc, err := xmlquery.Parse(strings.NewReader(content))
	if err != nil {
		return nil
	}

	urlNode := xmlquery.FindOne(doc, "//@cdnurl")
	if urlNode == nil || len(urlNode.InnerText()) == 0 {
		return nil
	}
	url := urlNode.InnerText()
	hashNode := xmlquery.FindOne(doc, "//@aeskey")
	if hashNode == nil || len(hashNode.InnerText()) == 0 {
		return nil
	}
	hash := hashNode.InnerText()

	data, err := GetBytes(url)
	if err == nil {
		return &common.BlobData{
			Name:   hash,
			Binary: data,
		}
	} else {
		return nil
	}
}

func download(resp *http.Response, err error) (*common.BlobData, error) {
	if err != nil {
		return nil, err
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	if data, err := io.ReadAll(resp.Body); err != nil {
		return nil, err
	} else {
		if len(data) == 0 {
			return nil, errors.New("file must be non-empty")
		}

		disposition := resp.Header.Get("Content-Disposition")
		if disposition != "" {
			if _, params, err := mime.ParseMediaType(disposition); err == nil {
				fileName := params["filename"]
				if decoded, err := url.QueryUnescape(fileName); err == nil {
					fileName = decoded
				}
				return &common.BlobData{
					Name:   fileName,
					Mime:   mimetype.Detect(data).String(),
					Binary: data,
				}, nil
			}
		}

		randBytes := make([]byte, 4)
		rand.Read(randBytes)
		fileName := hex.EncodeToString(randBytes)

		contentType := resp.Header.Get("Content-Disposition")
		if contentType != "" {
			return &common.BlobData{
				Name:   fileName + mimetype.Lookup(contentType).Extension(),
				Mime:   contentType,
				Binary: data,
			}, nil
		} else {
			mType := mimetype.Detect(data)
			return &common.BlobData{
				Name:   fileName + mType.Extension(),
				Mime:   mType.String(),
				Binary: data,
			}, nil
		}
	}
}

func GetBytes(url string) ([]byte, error) {
	reader, err := HTTPGetReadCloser(url)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = reader.Close()
	}()

	return io.ReadAll(reader)
}

type gzipCloser struct {
	f io.Closer
	r *gzip.Reader
}

func NewGzipReadCloser(reader io.ReadCloser) (io.ReadCloser, error) {
	gzipReader, err := gzip.NewReader(reader)
	if err != nil {
		return nil, err
	}

	return &gzipCloser{
		f: reader,
		r: gzipReader,
	}, nil
}

func (g *gzipCloser) Read(p []byte) (n int, err error) {
	return g.r.Read(p)
}

func (g *gzipCloser) Close() error {
	_ = g.f.Close()

	return g.r.Close()
}

func HTTPGetReadCloser(url string) (io.ReadCloser, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header["User-Agent"] = []string{UserAgent}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	log.Errorf("WTF respe: %+v", resp.Header)
	if strings.Contains(resp.Header.Get("Content-Encoding"), "gzip") {
		return NewGzipReadCloser(resp.Body)
	}

	return resp.Body, err
}
