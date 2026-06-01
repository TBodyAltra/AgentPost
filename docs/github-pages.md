# GitHub Pages 部署说明

站点地址：<https://tbodyaltra.github.io/AgentPost/>

## 正常发布

合并到 `main` 且改动包含 `docs/**` 或 `.github/workflows/pages.yml` 时，**Deploy GitHub Pages** workflow 会自动运行。

本地预览（不经过 Actions）：

```bash
python3 -m http.server 8765 --directory docs
```

浏览器打开 <http://127.0.0.1:8765/>。

## 常见失败：`in progress deployment`

若 Actions 报错类似：

```text
Deployment request failed ... due to in progress deployment.
Please cancel <commit-sha> first or wait for it to complete.
```

通常是上一次部署遇到 GitHub **502** 或被取消，Pages 后端仍把该提交标为「进行中」，阻塞后续发布。

当前 workflow 会在部署前通过 Pages API **列出并取消**所有 `deployment_in_progress` / `building` / `queued` / `pending` 的部署，等待约 45 秒后再发布；若仍失败，会再次取消阻塞项、等待约 90 秒后自动重试一次 `deploy-pages`。

**处理步骤（任选其一）：**

1. **重新运行 workflow**（推荐）  
   在 **Actions → Deploy GitHub Pages → Re-run all jobs** 即可（无需手动取消旧 SHA）。

2. **仓库管理员用 CLI 取消卡住的提交**（将 `<sha>` 换成报错里的 commit）：

   ```bash
   gh api -X POST \
     -H "Accept: application/vnd.github+json" \
     repos/TBodyAltra/AgentPost/pages/deployments/<sha>/cancel
   ```

3. **Settings → Pages**  
   确认 **Build and deployment → Source** 为 **GitHub Actions**（不要选 “Deploy from a branch / gh-pages”）。

4. 仍无法恢复时：暂时关闭 Pages，等待数分钟后再开启，并重新运行 workflow。

## 线上仍是旧页面

合并成功后若站点未更新：

- 等待 1～2 分钟再访问。
- 浏览器 **强制刷新**（Ctrl+Shift+R / Cmd+Shift+R）。
- 在 Actions 中确认最近一次 **Deploy GitHub Pages** 为绿色。
