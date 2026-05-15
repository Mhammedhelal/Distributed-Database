#include "mysql_client.hpp"
#include <stdexcept>
#include <sstream>

namespace db {

MySQLClient::MySQLClient(const std::string& host, int port,
                         const std::string& user, const std::string& password,
                         const std::string& database) {
    conn_ = mysql_init(nullptr);
    if (!conn_) throw std::runtime_error("mysql_init failed");

    if (!mysql_real_connect(conn_, host.c_str(), user.c_str(),
                            password.c_str(), database.c_str(),
                            port, nullptr, 0)) {
        std::string err = mysql_error(conn_);
        mysql_close(conn_);
        conn_ = nullptr;
        throw std::runtime_error("mysql_real_connect: " + err);
    }
    mysql_set_character_set(conn_, "utf8mb4");
}

MySQLClient::~MySQLClient() {
    if (conn_) mysql_close(conn_);
}

// exec runs a statement that returns no result set.
void MySQLClient::exec(const std::string& sql) {
    std::lock_guard<std::mutex> lk(mu_);
    if (mysql_query(conn_, sql.c_str())) {
        throw std::runtime_error(std::string("mysql_query: ") + mysql_error(conn_));
    }
}

// query runs a SELECT and returns rows as vector of string maps.
std::vector<Row> MySQLClient::query(const std::string& sql) {
    std::lock_guard<std::mutex> lk(mu_);
    if (mysql_query(conn_, sql.c_str())) {
        throw std::runtime_error(std::string("mysql_query: ") + mysql_error(conn_));
    }
    MYSQL_RES* res = mysql_store_result(conn_);
    if (!res) {
        if (mysql_field_count(conn_) == 0) return {}; // no result set
        throw std::runtime_error(std::string("mysql_store_result: ") + mysql_error(conn_));
    }
    struct ResGuard { MYSQL_RES* r; ~ResGuard(){ mysql_free_result(r); } } guard{res};

    unsigned int nfields = mysql_num_fields(res);
    MYSQL_FIELD* fields = mysql_fetch_fields(res);

    std::vector<Row> rows;
    MYSQL_ROW row;
    while ((row = mysql_fetch_row(res))) {
        unsigned long* lengths = mysql_fetch_lengths(res);
        Row r;
        for (unsigned int i = 0; i < nfields; ++i) {
            r[fields[i].name] = row[i] ? std::string(row[i], lengths[i]) : "";
        }
        rows.push_back(std::move(r));
    }
    return rows;
}

// escape returns a safely escaped string value for use in SQL literals.
std::string MySQLClient::escape(const std::string& s) {
    std::string buf(s.size() * 2 + 1, '\0');
    unsigned long len = mysql_real_escape_string(conn_, buf.data(), s.c_str(), s.size());
    buf.resize(len);
    return buf;
}

uint64_t MySQLClient::lastInsertId() {
    return (uint64_t)mysql_insert_id(conn_);
}

uint64_t MySQLClient::affectedRows() {
    return (uint64_t)mysql_affected_rows(conn_);
}

} // namespace db