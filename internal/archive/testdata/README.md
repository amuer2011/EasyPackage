# 独立 AES ZIP 样本

`pyzipper-aes128.zip`、`pyzipper-aes192.zip`、`pyzipper-aes256.zip` 由独立实现 pyzipper 0.4.0 / pycryptodomex 3.23.0 生成，用于验证 Go 解密的互操作性。

测试密码（公开的测试数据）：`测试-password!`

文件：`nested/你好.txt`，内容 `Independent AES fixture ✓`；`empty.txt` 为空文件。

使用 ZIP_DEFLATED 和 WZ_AES，分别设置 nbits=128/192/256。样本由开发验证脚本 `build/interop.py` 生成；运行普通 Go 测试无需 Python 或第三方库。该脚本还使用 pyzipper 成功读取了 Go 生成的 AES-256 ZIP（含中文路径、空文件）。
