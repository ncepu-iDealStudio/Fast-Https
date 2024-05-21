#!/bin/bash

# 定义配置文件路径
CONFIG_FILE="./config_dev/consts.go"

# 定义临时文件
TEMP_FILE="./config_dev/consts_temp.go"

# 替换字符串并构建函数
replace_and_build() {
    local old_string=$1
    local new_string=$2
    
    # 生成新的配置文件
    sed "s/$old_string/$new_string/g" $CONFIG_FILE > $TEMP_FILE

    # 备份原配置文件
    cp $CONFIG_FILE "${CONFIG_FILE}.bak"
    
    # 使用临时文件替换原文件
    mv $TEMP_FILE $CONFIG_FILE

    # 执行 go build
    go build

    # 恢复原配置文件
    mv "${CONFIG_FILE}.bak" $CONFIG_FILE
}

# 替换 engine_slaveC 为 engine_slaveA 并构建
replace_and_build "engine_slaveC" "engine_slaveA"

# 替换 engine_slaveC 为 engine_slaveB 并构建
replace_and_build "engine_slaveC" "engine_slaveB"