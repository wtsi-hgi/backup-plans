/*******************************************************************************
 * Copyright (c) 2025 Genome Research Ltd.
 *
 * Author: Michael Woolnough <mw31@sanger.ac.uk>
 *
 * Permission is hereby granted, free of charge, to any person obtaining
 * a copy of this software and associated documentation files (the
 * "Software"), to deal in the Software without restriction, including
 * without limitation the rights to use, copy, modify, merge, publish,
 * distribute, sublicense, and/or sell copies of the Software, and to
 * permit persons to whom the Software is furnished to do so, subject to
 * the following conditions:
 *
 * The above copyright notice and this permission notice shall be included
 * in all copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
 * EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
 * MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
 * IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY
 * CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT,
 * TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE
 * SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
 ******************************************************************************/

//nolint:gochecknoglobals
package db

var tables = [...]string{
	"CREATE TABLE IF NOT EXISTS `directories` (" +
		"`id` INTEGER PRIMARY KEY " + autoIncrement + ", " +
		"`directory` TEXT NOT NULL, " +
		"`directoryHash` " + hashColumnStart + "`directory`" + hashColumnEnd + ", " +
		"`claimedBy` TEXT NOT NULL, " +
		"`frequency` INTEGER NOT NULL, " +
		"`frozen` BOOLEAN DEFAULT FALSE, " +
		"`reviewDate` BIGINT NOT NULL, " +
		"`removeDate` BIGINT NOT NULL, " +
		"`melt` BIGINT NOT NULL DEFAULT 0, " +
		"`created` BIGINT NOT NULL, " +
		"`modified` BIGINT NOT NULL, " +
		"UNIQUE(`directoryHash`)" +
		");",

	"CREATE TABLE IF NOT EXISTS `rules` (" +
		"`id` INTEGER PRIMARY KEY " + autoIncrement + ", " +
		"`directoryID` INTEGER NOT NULL, " +
		"`type` INTEGER NOT NULL, " +
		"`metadata` TEXT NOT NULL, " +
		"`match` /*! MEDIUMTEXT -- */ TEXT\n/*! */ NOT NULL, " + // collection name if collection rule
		"`matchHash` " + hashColumnStart + "`match`" + hashColumnEnd + ", " +
		"`override` BOOLEAN DEFAULT FALSE, " +
		"`isCollection` BOOLEAN DEFAULT FALSE, " +
		"`created` BIGINT NOT NULL, " +
		"`modified` BIGINT NOT NULL, " +
		"UNIQUE(`directoryID`, `matchHash`), " +
		"FOREIGN KEY(`directoryID`) REFERENCES `directories`(`id`) ON DELETE CASCADE" +
		");",

	"CREATE TABLE IF NOT EXISTS `collections` (" +
		"`id` INTEGER PRIMARY KEY " + autoIncrement + ", " +
		"`name` TEXT NOT NULL, " +
		"`description` TEXT, " +
		"`created` BIGINT NOT NULL, " +
		"`modified` BIGINT NOT NULL " +
		");",

	"CREATE TABLE IF NOT EXISTS `collectionRules` (" +
		"`id` INTEGER PRIMARY KEY " + autoIncrement + ", " +
		"`collectionID` INTEGER NOT NULL, " +
		"`type` INTEGER NOT NULL, " +
		"`metadata` TEXT NOT NULL, " +
		"`match` /*! MEDIUMTEXT -- */ TEXT\n/*! */ NOT NULL, " +
		"`matchHash` " + hashColumnStart + "`match`" + hashColumnEnd + ", " +
		"`override` BOOLEAN DEFAULT FALSE, " +
		"`created` BIGINT NOT NULL, " +
		"`modified` BIGINT NOT NULL, " +
		"UNIQUE(`collectionID`, `matchHash`), " +
		"FOREIGN KEY(`collectionID`) REFERENCES `collections`(`id`) ON DELETE CASCADE" +
		");",
}

var tableNames = [...]string{"directories", "rules", "collections", "collectionRules"}

const (
	autoIncrement   = "/*! AUTO_INCREMENT -- */ AUTOINCREMENT\n/*! */"
	virtStart       = "/*! UNHEX(SHA2(*/"
	virtEnd         = "/*!, 0))*/"
	hashColumnStart = "/*! VARBINARY(32) -- */ TEXT\n/* */GENERATED ALWAYS AS (" + virtStart
	hashColumnEnd   = virtEnd + ") VIRTUAL /*! INVISIBLE */"

	tableCheck = "SELECT " +
		"COUNT(1) " +
		"FROM /*! `information_schema`.`tables` -- */ `sqlite_master`\n/*! */ " +
		"WHERE " +
		"/*! `table_schema` = DATABASE() -- */ `type` = 'table'\n/*! */ AND " +
		"/*! `table_name` -- */ `name`\n/*! */ = ?;"

	createDirectory = "INSERT INTO `directories` (" +
		"`directory`, " +
		"`claimedBy`, " +
		"`frequency`, " +
		"`frozen`, " +
		"`reviewDate`, " +
		"`removeDate`, " +
		"`created`, " +
		"`modified`" +
		") VALUES (?, ?, ?, ?, ?, ?, ?, ?);"
	createDirRule = "INSERT INTO `rules` " +
		"(`directoryID`, `type`, `metadata`, `match`, `override`, `isCollection`, `created`, `modified`) " +
		"VALUES (?, ?, ?, ?, ?, ?, ?, ?);"
	createCollection = "INSERT INTO `collections` " +
		"(`name`, `description`, `created`, `modified`) " +
		"VALUES (?, ?, ?, ?);"
	createCollectionRule = "INSERT INTO `collectionRules` " +
		"(`collectionID`, `type`, `metadata`, `match`, `override`, `created`, `modified`) " +
		"VALUES (?, ?, ?, ?, ?, ?, ?);"
	selectAllDirectories = "SELECT " +
		"`id`, " +
		"`directory`, " +
		"`claimedBy`, " +
		"`frequency`, " +
		"`frozen`, " +
		"`reviewDate`, " +
		"`removeDate`, " +
		"`melt`, " +
		"`created`, " +
		"`modified` " +
		"FROM `directories`;"
	selectAllRules = "SELECT " +
		"`id`, " +
		"`directoryID`, " +
		"`type`, " +
		"`metadata`, " +
		"`match`, " +
		"`override`, " +
		"`isCollection`, " +
		"`created`, " +
		"`modified` " +
		"FROM `rules`;"
	selectAllCollections = "SELECT " +
		"`id`, " +
		"`name`, " +
		"`description`, " +
		"`created`, " +
		"`modified` " +
		"FROM `collections`;"
	selectAllCollectionRules = "SELECT " +
		"`id`, " +
		"`collectionID`, " +
		"`type`, " +
		"`metadata`, " +
		"`match`, " +
		"`override`, " +
		"`created`, " +
		"`modified` " +
		"FROM `collectionRules`;"

	updateDirectory = "UPDATE `directories` SET " +
		"`claimedBy` = ?, " +
		"`modified` = ?, " +
		"`frequency` = ?, " +
		"`frozen` = ?, " +
		"`melt` = ?, " +
		"`reviewDate` = ?, " +
		"`removeDate` = ? " +
		"WHERE `id` = ?;"
	updateDirRule = "UPDATE `rules` SET " +
		"`type` = ?, " +
		"`metadata` = ?, " +
		"`match` = ?, " +
		"`isCollection` = ?, " +
		"`modified` = ? " +
		"WHERE `id` = ?;"
	updateCollection = "UPDATE `collections` SET " +
		"`name` = ?, " +
		"`description` = ?, " +
		"`modified` = ? " +
		"WHERE `id` = ?;"
	updateCollectionRule = "UPDATE `collectionRules` SET " +
		"`type` = ?, " +
		"`metadata` = ?, " +
		"`match` = ?, " +
		"`override` = ?, " +
		"`modified` = ? " +
		"WHERE `id` = ?;"
	refreezeDirectory = "UPDATE `directories` SET `melt` = 0 WHERE `id` = ?;"

	deleteDirectory      = "DELETE FROM `directories` WHERE `id` = ?;"
	deleteDirRule        = "DELETE FROM `rules` WHERE `id` = ?;"
	deleteCollection     = "DELETE FROM `collections` WHERE `id` = ?;"
	deleteCollectionRule = "DELETE FROM `collectionRules` WHERE `id` = ?;"
)
