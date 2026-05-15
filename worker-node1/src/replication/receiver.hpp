#pragma once
#include <string>
#include <functional>
#include "../db/mysql_client.hpp"
#include <nlohmann/json.hpp>

namespace replication {

using json = nlohmann::json;

// applyEntry applies a single WAL entry to the local MySQL database.
// Returns {ok, error_message}.
inline std::pair<bool, std::string> applyEntry(db::MySQLClient& db, const json& entry) {
    try {
        std::string op = entry.value("op", "");
        std::string database = entry.value("database", "");
        std::string table = entry.value("table", "");

        // Always switch to the correct database first
        if (!database.empty()) {
            db.exec("CREATE DATABASE IF NOT EXISTS `" + database + "`");
            db.exec("USE `" + database + "`");
        }

        if (op == "CREATE_DB") {
            // Already handled above
        } else if (op == "DROP_DB") {
            db.exec("DROP DATABASE IF EXISTS `" + database + "`");

        } else if (op == "CREATE_TABLE") {
            std::string cols_sql = "`id` INT AUTO_INCREMENT PRIMARY KEY";
            for (auto& col : entry["cols"]) {
                cols_sql += ", `" + col["name"].get<std::string>() + "` "
                          + col["type"].get<std::string>();
            }
            db.exec("CREATE TABLE IF NOT EXISTS `" + table + "` (" + cols_sql + ")");

        } else if (op == "DROP_TABLE") {
            db.exec("DROP TABLE IF EXISTS `" + table + "`");

        } else if (op == "INSERT") {
            auto data = entry["data"];
            std::string cols, vals;
            bool first = true;
            for (auto& [k, v] : data.items()) {
                if (!first) { cols += ","; vals += ","; }
                first = false;
                cols += "`" + k + "`";
                std::string sv = v.is_string() ? v.get<std::string>() : v.dump();
                vals += "'" + db.escape(sv) + "'";
            }
            db.exec("INSERT INTO `" + table + "` (" + cols + ") VALUES (" + vals + ")");

        } else if (op == "UPDATE") {
            auto data = entry["data"];
            std::string set_clause;
            bool first = true;
            for (auto& [k, v] : data.items()) {
                if (!first) set_clause += ",";
                first = false;
                std::string sv = v.is_string() ? v.get<std::string>() : v.dump();
                set_clause += "`" + k + "`='" + db.escape(sv) + "'";
            }
            std::string where = entry.value("where", "");
            std::string sql = "UPDATE `" + table + "` SET " + set_clause;
            if (!where.empty()) {
                // Replace ? with actual value from where_args[0]
                auto args = entry.value("where_args", json::array());
                if (!args.empty()) {
                    std::string arg = args[0].is_string() ? args[0].get<std::string>() : args[0].dump();
                    size_t pos = where.find('?');
                    if (pos != std::string::npos)
                        where.replace(pos, 1, "'" + db.escape(arg) + "'");
                }
                sql += " WHERE " + where;
            }
            db.exec(sql);

        } else if (op == "DELETE") {
            std::string where = entry.value("where", "");
            std::string sql = "DELETE FROM `" + table + "`";
            if (!where.empty()) {
                auto args = entry.value("where_args", json::array());
                if (!args.empty()) {
                    std::string arg = args[0].is_string() ? args[0].get<std::string>() : args[0].dump();
                    size_t pos = where.find('?');
                    if (pos != std::string::npos)
                        where.replace(pos, 1, "'" + db.escape(arg) + "'");
                }
                sql += " WHERE " + where;
            }
            db.exec(sql);

        } else {
            return {false, "unknown op: " + op};
        }

        return {true, ""};
    } catch (const std::exception& e) {
        return {false, e.what()};
    }
}

} // namespace replication