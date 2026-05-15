cmake_minimum_required(VERSION 3.16)
project(worker_node1 CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_BUILD_TYPE Release)

# ── Find dependencies ─────────────────────────────────────────────────────────
find_package(OpenSSL REQUIRED)
find_package(Threads REQUIRED)

# MySQL connector via pkg-config or manual path
find_library(MYSQL_LIB NAMES mysqlclient REQUIRED)
find_path(MYSQL_INCLUDE_DIR mysql/mysql.h REQUIRED)

# nlohmann/json (header-only — either installed or FetchContent)
find_package(nlohmann_json QUIET)
if(NOT nlohmann_json_FOUND)
    include(FetchContent)
    FetchContent_Declare(json
        URL https://github.com/nlohmann/json/releases/download/v3.11.3/json.hpp
        DOWNLOAD_NO_EXTRACT TRUE
        SOURCE_DIR ${CMAKE_BINARY_DIR}/nlohmann
    )
    FetchContent_MakeAvailable(json)
    add_library(nlohmann_json INTERFACE)
    target_include_directories(nlohmann_json INTERFACE ${CMAKE_BINARY_DIR})
endif()

# ── Sources ───────────────────────────────────────────────────────────────────
add_executable(worker_node1
    src/main.cpp
    src/db/mysql_client.cpp
)

target_include_directories(worker_node1 PRIVATE
    ${CMAKE_SOURCE_DIR}/include          # crow_all.h
    ${CMAKE_SOURCE_DIR}/src              # local headers
    ${MYSQL_INCLUDE_DIR}
    ${OPENSSL_INCLUDE_DIR}
)

target_link_libraries(worker_node1 PRIVATE
    ${MYSQL_LIB}
    OpenSSL::SSL
    OpenSSL::Crypto
    Threads::Threads
    nlohmann_json
)

# Crow requires Boost.Asio or standalone Asio
find_package(Boost COMPONENTS system QUIET)
if(Boost_FOUND)
    target_link_libraries(worker_node1 PRIVATE Boost::system)
else()
    # Standalone Asio (header-only)
    find_path(ASIO_INCLUDE_DIR asio.hpp)
    if(ASIO_INCLUDE_DIR)
        target_include_directories(worker_node1 PRIVATE ${ASIO_INCLUDE_DIR})
        target_compile_definitions(worker_node1 PRIVATE CROW_USE_STANDALONE_ASIO)
    else()
        message(WARNING "Neither Boost.Asio nor standalone Asio found. "
                        "Install libboost-dev or libasio-dev.")
    endif()
endif()

install(TARGETS worker_node1 DESTINATION bin)