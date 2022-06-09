# Rclone for Vault

Experimental support for Internet Archive Vault Digital Preservation System in
Rclone. This is work-in-progress and we are happy about feedback.

## Configuration

In you rclone config, add the following section.

```
[vault]
type = vault
username = admin
password = admin
endpoint = http://localhost:8000/api
```

## Examples

Note: Most examples show abbreviated outputs.

### Quota and Usage

* [x] about

```
$ rclone about vault:/
Total:   1 TiB
Used:    2.891 GiB
Free:    1021.109 GiB
Objects: 19.955k
```

### Listing files

* [x] ls
* [x] lsl
* [x] lsf
* [x] lsd, lsd -R
* [x] lsjson

```shell
$ rclone ls vault:/ | head -10
        8 C00/VERSION
        0 C1/abc.txt
     3241 C123/about.go
     4511 C123/backend.go
     3748 C123/bucket.go
     3683 C123/bucket_test.go
     2416 C123/cat.go
     2829 C123/copy.go
    10913 C123/help.go
      886 C123/ls.go

$ rclone lsl vault:/ | head -10
        0 2022-06-08 23:49:10.000000000 C1/abc.txt
        8 2022-05-31 16:17:21.000000000 C00/VERSION
     3241 2022-05-31 17:13:45.000000000 C123/about.go
     4511 2022-05-31 17:17:10.000000000 C123/backend.go
     3748 2022-05-31 18:18:36.000000000 C123/bucket.go
     3683 2022-05-31 18:20:44.000000000 C123/bucket_test.go
     2416 2022-05-31 17:18:42.000000000 C123/cat.go
     2829 2022-05-31 17:09:35.000000000 C123/copy.go
    10913 2022-05-31 17:27:17.000000000 C123/help.go
      886 2022-05-31 17:06:59.000000000 C123/ls.go


$ rclone lsf vault:/ | head -10
.Trash-1000/
C00/
C1/
C123/
C40/
C41/
C42/
C43/
C50/
C51/

$ rclone lsd vault:/ | head -10
           0 2022-05-31 16:05:24         0 .Trash-1000
           0 2022-05-31 16:17:05         2 C00
           0 2022-06-08 23:49:06         1 C1
           0 2022-05-31 17:06:59        25 C123
           0 2022-06-07 15:27:55         1 C40
           0 2022-06-07 15:35:33         1 C41
           0 2022-06-07 15:44:15         1 C42
           0 2022-06-08 11:20:18         1 C43
           0 2022-06-08 13:09:09         1 C50
           0 2022-06-08 14:34:18         2 C51

$ rclone lsd -R vault:/C40
           0 2022-06-07 15:33:32         1 myblog
           0 2022-06-07 15:33:36         0 myblog/templates

$ rclone lsjson vault:/ | head -10
[
{"Path":".Trash-1000","Name":".Trash-1000","Size":0,"MimeType":"inode/directory","ModTime":"2022-05-31T14:05:24Z","IsDir":true,"ID":"1.12"},
{"Path":"C00","Name":"C00","Size":0,"MimeType":"inode/directory","ModTime":"2022-05-31T14:17:05Z","IsDir":true,"ID":"1.25"},
{"Path":"C1","Name":"C1","Size":0,"MimeType":"inode/directory","ModTime":"2022-06-08T21:49:06Z","IsDir":true,"ID":"1.38600"},
{"Path":"C123","Name":"C123","Size":0,"MimeType":"inode/directory","ModTime":"2022-05-31T15:06:59Z","IsDir":true,"ID":"1.48"},
{"Path":"C40","Name":"C40","Size":0,"MimeType":"inode/directory","ModTime":"2022-06-07T13:27:55Z","IsDir":true,"ID":"1.665"},
{"Path":"C41","Name":"C41","Size":0,"MimeType":"inode/directory","ModTime":"2022-06-07T13:35:33Z","IsDir":true,"ID":"1.674"},
{"Path":"C42","Name":"C42","Size":0,"MimeType":"inode/directory","ModTime":"2022-06-07T13:44:15Z","IsDir":true,"ID":"1.683"},
{"Path":"C43","Name":"C43","Size":0,"MimeType":"inode/directory","ModTime":"2022-06-08T09:20:18Z","IsDir":true,"ID":"1.698"},
{"Path":"C50","Name":"C50","Size":0,"MimeType":"inode/directory","ModTime":"2022-06-08T11:09:09Z","IsDir":true,"ID":"1.713"},
...
```

### Creating Collections and Folder

Collections and folders are handled transparently (e.g. first path component
will be the name of the collection, and anything below: folders).

* [x] mkdir

```shell
$ rclone mkdir vault:/X1

$ rclone mkdir vault:/X2/a/b/c
```

### Uploading single files and trees

* [ ] copy
* [ ] copyto
* [ ] copyurl

### Sync

* [ ] sync

### Downloading files and trees

* [ ] copy
