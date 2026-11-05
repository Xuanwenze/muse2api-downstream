# 服务端选号补丁（muse2api）

上游 muse2api 的公开 API（`/v1/images/tasks`、`/v1/images/generations`、`/v1/images/edits`、`/v1/videos`）目前不接受 `account_id` 参数，只有内部引擎支持指定账号。本补丁把 `account_id` 从请求层直通到底层账号池，让客户端可以固定某个账号或轮询使用。

## 打补丁

在 muse2api 服务器上（以 v1.6.7 验证通过）：

```bash
cd ~/muse2api
cp app.py app.py.bak-$(date +%Y%m%d)
git apply server-patch/account-select.patch
sudo docker-compose up -d --build   # app.py 是 COPY 进镜像的，改完必须重新构建
```

验证：

```bash
curl -s http://127.0.0.1:18610/openapi.json | grep -o '"account_id"'
# ImageRequest 与 VideoRequest 里都应出现 account_id
```

## 行为说明

- `account_id` 是**首选**账号，不是绝对锁定：该账号失败时，服务器会换号重试一次
- 不传 `account_id` 时走服务器默认调度（视频优先上次出过视频的账号）
- 指定不存在或已禁用的账号会返回错误（`指定账号 … 不存在或已禁用`）

## 维护

- 上游升级会覆盖 `app.py`，升级后重新打一次补丁并重启即可
- 回滚：`git checkout -- app.py`（或恢复 `app.py.bak-*`），再重新构建

## 补丁内容

`account-select.patch`（共 14 处）：给 `ImageRequest` / `VideoRequest` 增加 `account_id: str | None` 字段，并在 `create_image_task` / `images_generations` / `images_edits` / `create_video` 四个入口把该参数透传给 `_run_generation` / `_queue_image`。
