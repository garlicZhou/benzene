
Linux是多用户多任务操作系统。在一个Linxu系统上，用户一般情况下一定有两类：root和普通用户，可能有多个以上。

```shell
#!/bin/bash
```

一个规范的Shell脚本在第一行指出由哪个程序（解释器）来执行脚本的内容。

```shell
unset -v progdir
```

删除变量 progdir，参数-v代表仅删除变量，-f代表仅删除函数。

```shell
case "${0}" in
*/*) progdir="${0%/*}";;
*) progdir=.;;
esac
```

${0} is the first argument of the script, i.e. the script name or path. If you launch your script as path/to/script.sh, then ${0} will be exactly that string: path/to/script.sh.

The %/* part modifies the value of ${0}. It means: take all characters until / followed by a file name. In the example above, ${0%/*} will be path/to.

The options that we understand are then listed and followed by a right bracket, as hello) and bye). `*/*)` denotes a string including a `/`.

;; denotes break 

*) denotes default


```shell
${HMY_PATH+set}
```

It is a form of bash parameter-substitution that will evaluate to "set" if $HMY_PATH has been set and null otherwise.

Reference: https://stackoverflow.com/questions/8059196/what-does-variableset-mean


```shell
if [ ! -d $directory ]
```

`-d` is a operator to test if the given directory exists or not. Thus, the above shell script will be true if the $directory does not exist.

Reference: https://stackoverflow.com/questions/37403759/what-is-the-meaning-of-d-in-this-bash-command


```shell
: ${OPENSSL_DIR="/usr/local/opt/openssl"}
```

赋值而已，例如对于: ${VAR:=DEFAULT}，当变量VAR没有声明或者为NULL时，将VAR设置为默认值DEFAULT，但如果不在前面加上:命令，那么就会把${VAR:=DEFAULT}本身当做一个命令来执行，报错是肯定的。

Reference: https://blog.csdn.net/honghuzhilangzixin/article/details/7073312


```shell
export CGO_CFLAGS="-I${BLS_DIR}/include -I${MCL_DIR}/include"
```

export命令用于设置环境变量。

子进程只会继承父进程的环境变量，子进程不会继承父进程的自定义变量。那么你原本bash中的自定义变量在进入子进程后就会消失不见，一直到你离开子进程并回到原本的父进程后，这个变量才会出现。除非把自定义变量设置为环境变量`export name`。

Q: 为什么setup_bls_build_flags.sh里面要用export，它都已经没有子进程了?
A: 因为go_executable_build.sh中是用.（点符号）读入setup_bls_build_flags.sh的，即在当前Shell中执行加载并执行的相关脚本文件的命令及语句，而不是产生一个子Shell来执行。

Reference: https://blog.csdn.net/weixin_38256474/article/details/90713950

```bash
$ echo '{"url": "mozillazg.com"}' |jq .
{
  "url": "mozillazg.com"
}
```

输出原始的 JSON 数据，默认不指定 filter 就会原样输出，也可以使用 . 过滤器。
