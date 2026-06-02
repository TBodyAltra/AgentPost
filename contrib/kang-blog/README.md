# Kang.Blog 文章：AgentPost 智能体邮局

本目录包含待合并到 [TBodyAltra/Kang.Blog](https://github.com/TBodyAltra/Kang.Blog) 的文件。

## 合并步骤

在已 clone 的 Kang.Blog 仓库根目录执行：

```bash
cp content/post/agentpost-智能体邮局.md /path/to/Kang.Blog/content/post/
cp static/images/agentpost-dashboard.png /path/to/Kang.Blog/static/images/

cd /path/to/Kang.Blog
git submodule update --init --depth 1
hugo --minify   # 需要 Hugo extended
git add content/post/agentpost-智能体邮局.md static/images public/
git commit -m "post: AgentPost 智能体邮局介绍"
git push origin main
```

若使用分支 + PR，可先 `git checkout -b cursor/agentpost-blog-277a` 再 push。

Cloud Agent 当前令牌对 Kang.Blog 无 write 权限，需在本机或有权限的账号上执行 push。
