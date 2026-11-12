package web

// Simplified Chinese text of the "add a website" flow.
func init() {
	for k, v := range map[string]string{
		"page.sitenew":     "添加网站",
		"sites.new":        "添加网站",
		"new.title":        "添加网站",
		"new.sub":          "选择要创建的网站类型。",
		"new.back":         "返回",
		"new.create":       "创建网站",
		"new.desc.php":     "PHP 网站，例如 WordPress 或 Laravel。每个网站有自己的 PHP 进程池。",
		"new.desc.python":  "Python 应用（Flask、Django、FastAPI 等），在自己的虚拟环境中作为服务运行，nginx 将请求转发给它。",
		"new.desc.node":    "Node.js 应用，作为独立服务运行，nginx 将请求转发给它。",
		"new.desc.static":  "仅提供现有的 HTML、CSS 和图片文件。",
		"new.desc.proxy":   "将所有请求转发到另一个地址，例如运行在其他服务器上的应用。",
		"f.hosts.hint":     "一个域名，例如 example.com。之后可在概览中配置 www，并在 SSL 标签页签发证书。",
		"f.user":           "网站账户",
		"f.user.hint":      "本网站的主 SSH/SFTP 账户。根据域名自动填写，可修改。",
		"f.password":       "账户密码",
		"f.password.hint":  "用于 SSH/SFTP。创建后，密码仅在结果页面显示一次：请保存。",
		"f.password.gen":   "生成新密码",
		"f.password.empty": "留空则自动生成",
		"res.access":       "SSH/SFTP 登录\n  服务器：   %s\n  用户名：   %s\n  密码：     %s",
		"err.password":     "密码无效：需要 12 到 64 个字符，不能包含空白、引号、反斜杠或反引号。",
		"err.sitename":     "无法从域名生成网站名称，请检查域名。",
		"err.kind":         "网站类型无效。",
		"err.nohosts":      "请至少输入一个域名。",
		"err.userreserved": "账户名 %q 为系统保留名称，请选择其他名称。",
		"err.userexists":   "本服务器上已存在账户 %q，请选择其他名称。",
		"err.userused":     "账户 %q 已被网站 %q 使用，请选择其他名称。",
		"err.siteused":     "已存在名为 %q 的网站。",
		"err.phpmissing":   "未安装 PHP %q。安装命令：mpn plan -php <version> -apply",
		"err.nodemissing":  "未安装 Node.js %q。安装命令：mpn plan -node <version> -apply",
		"new.nophp":        "本机未安装任何 PHP 版本。安装命令：mpn plan -php 8.4 -apply",
		"new.nonode":       "本机未安装任何 Node.js 版本。安装命令：mpn plan -node 22 -apply",
		"err.exitstatus":   "命令执行失败，请查看下方结果详情。",
	} {
		zh[k] = v
	}
}
