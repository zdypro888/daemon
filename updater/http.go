package main

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io/ioutil"
	"net"
	"net/http"
	"time"

	"golang.org/x/net/http2"
)

//证书
const (
	CACertPEM string = `-----BEGIN CERTIFICATE-----
MIIDtTCCAp2gAwIBAgIITWWCIQf8/VIwDQYJKoZIhvcNAQELBQAwVjELMAkGA1UE
BhMCU0cxEjAQBgNVBAgTCVNpbmdhcG9yZTESMBAGA1UEBxMJU2luZ2Fwb3JlMREw
DwYDVQQKEwhQcm8gSW5jLjEMMAoGA1UEAxMDUHJvMB4XDTIxMDEwNjEyMDUyM1oX
DTIzMDEwNjEyMDUyM1owVjELMAkGA1UEBhMCU0cxEjAQBgNVBAgTCVNpbmdhcG9y
ZTESMBAGA1UEBxMJU2luZ2Fwb3JlMREwDwYDVQQKEwhQcm8gSW5jLjEMMAoGA1UE
AxMDUHJvMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA1GtNXNNGI7Ue
mhy//llunVC7YOrkKMXjACsASVOl7S7wZwzgng5me7u+0ODDxw5/8Kv/6Vu8xDdT
5FXCB1P6YoaD571fKqEua3kAU//qv966jL77tDAVgXO7ADNzD0PJAZv8oOzKUd6z
T6JfuA92q+nC6meGlaTFwHS7h8vjOguclri7ODsS++ZAzwdnvNdycl2GZCagpJq0
DYs/ddsDI775VyoshpBYRPHsCIh7/YsM+OT4Fj6bVg8YUtfvEsFkGKUT3mG+Dhfs
1AJmNQ83hn8A5rdkdEOySSpUIVAaAagufYfOgBUTTyQ0aeQnkNqZPpvNIwKz0rHD
/P3erNOLGQIDAQABo4GGMIGDMA4GA1UdDwEB/wQEAwIChDAdBgNVHSUEFjAUBggr
BgEFBQcDAgYIKwYBBQUHAwEwEgYDVR0TAQH/BAgwBgEB/wIBADAdBgNVHQ4EFgQU
czcFHyA31HG31kmIc02qbI69IjUwHwYDVR0jBBgwFoAUczcFHyA31HG31kmIc02q
bI69IjUwDQYJKoZIhvcNAQELBQADggEBAMQOmtQXb6NyNkOCccc+5mky4+t6qprk
Zs/ydhrVUwQqQP9EFS/uIA4JU2ws4cxFrkps+imGdm5m5/30jJGZRY9a6CWiPWfw
NnDxx0BqkLeG/N34oueAsQKp2BLlN2s5n0tlLaVJxKVYZ5zaZ1QaAibhHW1Av4TJ
kVh3nbVoJ5jBLrPwgwsQBKTzTmxfc69OGQIvSSyQyLHD1QutanSu9d3KiVYVFt7P
pmnSP+rUbQAWhDb5gttRaDHAnVozSueazVCOhQzEjVNoQPiDeBzNrS2hxKXJphpC
aEpZn03hI4md8gSPg7/NE+3rXtRBgJVbZMFvMHArqBmM61elVw9PdfI=
-----END CERTIFICATE-----
-----BEGIN PRIVATE KEY-----
MIIEpQIBAAKCAQEA1GtNXNNGI7Uemhy//llunVC7YOrkKMXjACsASVOl7S7wZwzg
ng5me7u+0ODDxw5/8Kv/6Vu8xDdT5FXCB1P6YoaD571fKqEua3kAU//qv966jL77
tDAVgXO7ADNzD0PJAZv8oOzKUd6zT6JfuA92q+nC6meGlaTFwHS7h8vjOguclri7
ODsS++ZAzwdnvNdycl2GZCagpJq0DYs/ddsDI775VyoshpBYRPHsCIh7/YsM+OT4
Fj6bVg8YUtfvEsFkGKUT3mG+Dhfs1AJmNQ83hn8A5rdkdEOySSpUIVAaAagufYfO
gBUTTyQ0aeQnkNqZPpvNIwKz0rHD/P3erNOLGQIDAQABAoIBAA777r4gjS8RpLH8
WzLG/j2Mp1sj1qployis391MUEUV7ZFnYCTmISaTTNeRM15EUJQanffJJ9yzhnBx
+DjqHJx8nqtnOWJZclvUckh6ogWc4Y3yHvFL/whdsJBIENK/1lsNtNlpOrBhxEZW
zue994ITAFPmr6C4udZkpaHjqQi8Dg3OrAkfUf3K+iGxfEMngQ2dPMxQl4YfC46a
5DJ9tke1JDv79QNjF6zMcXHj6jtUyTxlZBbjVEY1ZxPw3OUbQ3ffnvkzVBlxjl0p
35yYPGQMeYduZsbL5WOoZ7PlUFLqW0tVWPR5EV3lfNne/VOt0aiYpGjCPgNFiXFF
4Sa8Uw0CgYEA7ARbKqAsiZW3JHzE0jGvz2+ZydhQNFpPHWCE+YfiRSEIHevGB1Yi
LJ68rhB7N5hGP/DwZzbJxKISmKYuuHMbdjjZJ6VhxqUp4Poy4u4bijvaNcHNFAoW
yEuIZQIMyCLxpWldYfSYpznGFpjwS/wfxSwaKeiJ3unnV1KCy2N6hL8CgYEA5md2
/aOqd54PN/jLeW7Qy/tiMqGUF6MkH7GYh7/niH6tHUD2jsVUJAN0FF0XgT1Ja8gv
A+1jRpY2WZzTsgBhSZFvgoVmq8ekBSp6sLbfr+8PeECpg3W2LdmmZkXp/L7VpQ96
iT09TaEIPk05O2YKVCHiUpRruGsAw3MWb1dxLicCgYEAtiJ6dD+dfyORbM/4V7lO
UoduJ80NwAj9Ss9kbuiFHhHqoKSFcr3uq35oXu+LFxElDU0TSKOIO31TWofMQD1c
MPSX6DeBZ/mngt2yDVvw1tFviNKhP1i10iYwALr/QCdvUdYo4WIPt+Umz+OAdTMB
FXj+S98PHn5lMAcVtn1zXCMCgYEA3cQom+m0Yn4YV994ueEXx76mveUYDchRNNBT
6BWmXZLQPaARsUntutw4FoGj5hl/WebMmhMbww1CMu7oNCR5f74kfpS4Rg9aqD5C
6WSb2VNYqH5UqtvaBjfAGiChH0zvhnhnkUEIiHe+33ik5a9JscELfkCtjkwv5/AW
YATiQ3ECgYEA5T6yTZYSo3X/bXlBXJOq6gQM0+RhG08cLGiXdAjFd1j8p1vMqLV4
uVW9d2iaAjW1jAtctKXdIiPLU/q2wbQrl1YklM20J7kyJl4VfiUDHTOM5aY9/igc
VjLPW8nG7F0P+MyLEwrs/0NCoVeNcDmYJ9y/8+HplbtX9NFS0R4dZtU=
-----END PRIVATE KEY-----
`
)

func httpRequest(url string) ([]byte, error) {
	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM([]byte(CACertPEM)) {
		return nil, errors.New("can not get root CA")
	}
	transport := &http.Transport{
		Dial: (&net.Dialer{
			Timeout: 20 * time.Second,
		}).Dial,
		ExpectContinueTimeout: 10 * time.Second,
		ResponseHeaderTimeout: 20 * time.Second,
		TLSHandshakeTimeout:   30 * time.Second,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: false,
			MinVersion:         tls.VersionTLS11,
			MaxVersion:         tls.VersionTLS13,
			RootCAs:            roots,
		},
	}
	if err = http2.ConfigureTransport(transport); err != nil {
		return nil, err
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   120 * time.Second,
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	return ioutil.ReadAll(response.Body)
}
