# 虚拟磁盘上传支持 .vfd 说明

本次修复补齐了虚拟磁盘上传的后端后缀白名单，现已允许上传 `.vfd` 文件。

涉及位置：

- `server/handler/vm_export_import.go`
- `server/handler/user_storage.go`
- `server/service/upload_wire.go`

说明：

- 前端页面本身已提示支持 `.vfd`，无需额外修改。
- 该修复同时覆盖普通上传与分片/封装上传场景中的磁盘文件后缀校验。
