#pragma once
#include <mysql/mysql.h>
#include <string>
#include <vector>
#include <map>
#include <mutex>

namespace db {

using Row = std::map<std::string, std::string>;

class MySQLClient {
public:
    MySQLClient(const std::string& host, int port,
                const std::string& user, const std::string& password,
                const std::string& database);
    ~MySQLClient();

    void exec(const std::string& sql);
    std::vector<Row> query(const std::string& sql);
    std::string escape(const std::string& s);
    uint64_t lastInsertId();
    uint64_t affectedRows();

private:
    MYSQL* conn_{nullptr};
    std::mutex mu_;
};

} // namespace db