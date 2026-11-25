# moon-app-template

推送到 `main` 即自动构建并部署到 moon，地址 `https://<仓库名>.moonl.cloud`。

## 用这个模板新建项目

```bash
gh repo create HinaKalo/<名称> --private --template HinaKalo/moon-app-template --clone
```

然后改 `compose.yaml` 里的端口（moon 上唯一，建议 18100–18999），写代码，推送到 `main`。

## 文件

- `Dockerfile`：多阶段构建，产物为 distroless 非 root 镜像
- `compose.yaml`：部署编排；`x-moon` 声明公网入口与共享数据库/缓存
- `.github/workflows/deploy.yml`：调用 `HinaKalo/moon-infra` 的中央流水线

## 本地运行

```bash
go run .
```

不设置 `DATABASE_URL` / `REDIS_URL` 时，应用跳过数据库和缓存。

规则与平台说明见 [HinaKalo/moon-infra](https://github.com/HinaKalo/moon-infra)。
