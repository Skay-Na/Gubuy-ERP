-- MySQL dump 10.13  Distrib 8.0.46, for Linux (x86_64)
--
-- Host: localhost    Database: erp_db
-- ------------------------------------------------------
-- Server version	8.0.46

/*!40101 SET @OLD_CHARACTER_SET_CLIENT=@@CHARACTER_SET_CLIENT */;
/*!40101 SET @OLD_CHARACTER_SET_RESULTS=@@CHARACTER_SET_RESULTS */;
/*!40101 SET @OLD_COLLATION_CONNECTION=@@COLLATION_CONNECTION */;
/*!50503 SET NAMES utf8mb4 */;
/*!40103 SET @OLD_TIME_ZONE=@@TIME_ZONE */;
/*!40103 SET TIME_ZONE='+00:00' */;
/*!40014 SET @OLD_UNIQUE_CHECKS=@@UNIQUE_CHECKS, UNIQUE_CHECKS=0 */;
/*!40014 SET @OLD_FOREIGN_KEY_CHECKS=@@FOREIGN_KEY_CHECKS, FOREIGN_KEY_CHECKS=0 */;
/*!40101 SET @OLD_SQL_MODE=@@SQL_MODE, SQL_MODE='NO_AUTO_VALUE_ON_ZERO' */;
/*!40111 SET @OLD_SQL_NOTES=@@SQL_NOTES, SQL_NOTES=0 */;

--
-- Table structure for table `employees`
--

DROP TABLE IF EXISTS `employees`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `employees` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  `name` varchar(100) COLLATE utf8mb4_general_ci NOT NULL COMMENT '员工姓名',
  `phone` varchar(20) COLLATE utf8mb4_general_ci DEFAULT NULL COMMENT '联系电话',
  `emp_no` varchar(50) COLLATE utf8mb4_general_ci NOT NULL COMMENT '工号',
  `base_salary` decimal(10,2) NOT NULL DEFAULT '0.00' COMMENT '基本底薪',
  `commission_rate` decimal(5,4) NOT NULL DEFAULT '0.0000' COMMENT '提成比例',
  `pin_code` varchar(10) COLLATE utf8mb4_general_ci DEFAULT NULL COMMENT '终端登录PIN码',
  `entry_date` date DEFAULT NULL COMMENT '入职时间',
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `is_outsourced` tinyint(1) DEFAULT '0' COMMENT '是否为外包人员',
  `outsourced_type` varchar(20) COLLATE utf8mb4_general_ci DEFAULT '' COMMENT '外包类型：install(安装), delivery(送货)',
  `role` varchar(50) COLLATE utf8mb4_general_ci DEFAULT 'sales' COMMENT '角色分类(admin/sales/installer等)',
  `pinyin` varchar(255) COLLATE utf8mb4_general_ci DEFAULT NULL COMMENT '名字全拼',
  `pinyin_initial` varchar(100) COLLATE utf8mb4_general_ci DEFAULT NULL COMMENT '名字首字母',
  `employment_status` varchar(20) COLLATE utf8mb4_general_ci NOT NULL DEFAULT 'active' COMMENT '任职状态 active/offboarding/departed',
  `last_working_date` date DEFAULT NULL COMMENT '最后工作日',
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_employees_emp_no` (`emp_no`),
  KEY `idx_employees_deleted_at` (`deleted_at`),
  KEY `idx_employees_employment_status` (`employment_status`),
  KEY `idx_employees_last_working_date` (`last_working_date`)
) ENGINE=InnoDB AUTO_INCREMENT=14 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Dumping data for table `employees`
--
-- WHERE:  1=1

/*!40000 ALTER TABLE `employees` DISABLE KEYS */;
INSERT INTO `employees` (`id`, `name`, `phone`, `emp_no`, `base_salary`, `commission_rate`, `pin_code`, `entry_date`, `created_at`, `updated_at`, `deleted_at`, `is_outsourced`, `outsourced_type`, `role`, `pinyin`, `pinyin_initial`, `employment_status`, `last_working_date`) VALUES (1,'李梅梅','13639497316','001',2000.00,0.0100,'2013','2026-05-01','2026-05-09 19:47:46.479','2026-07-03 15:32:18.091',NULL,0,'','sales','limeimei','lmm','active',NULL),(2,'刘玉艳','18295714097','002',2000.00,0.0100,'0526','2026-05-01','2026-05-11 10:57:33.770','2026-07-03 15:32:18.114','2026-10-03 13:10:02.758',0,'','sales','liuyuyan','lyy','active',NULL),(3,'任志鹏','17835755689','003',2000.00,0.0100,'2627','2026-05-01','2026-05-11 16:11:49.501','2026-07-03 15:32:18.139',NULL,0,'','sales','renzhipeng','rzp','active',NULL),(4,'杨少利','15364847269','WB1780911232060',0.00,0.0000,'',NULL,'2026-06-08 17:34:13.650','2026-07-03 15:32:18.163',NULL,1,'install','sales','yangshaoli','ysl','active',NULL),(5,'李阳阳','17835340721','WB1780911255378',0.00,0.0000,'',NULL,'2026-06-08 17:34:56.754','2026-07-03 15:32:18.188',NULL,1,'install','sales','liyangyang','lyy','active',NULL),(6,'军兵','15296776490','WB1780911296713',0.00,0.0000,'',NULL,'2026-06-08 17:35:06.928','2026-07-03 15:32:18.213',NULL,1,'install','sales','junbing','jb','active',NULL),(7,'明刚','15035035022','WB1780911307009',0.00,0.0000,'',NULL,'2026-06-08 17:35:17.027','2026-07-03 15:32:18.239',NULL,1,'install','sales','minggang','mg','active',NULL),(8,'常毅','15535981318','WB1780911325859',0.00,0.0000,'',NULL,'2026-06-08 17:35:45.592','2026-07-03 15:32:18.265',NULL,1,'install','sales','changyi','cy','active',NULL),(9,'范晨奔','15333096468','WB1780911345632',0.00,0.0000,'',NULL,'2026-06-08 17:36:07.836','2026-07-03 15:32:18.291',NULL,1,'install','sales','fanchenben','fcb','active',NULL),(10,'文斌','13834487971','WB1780920094363',0.00,0.0000,'',NULL,'2026-06-08 20:01:40.728','2026-07-03 15:32:18.320',NULL,1,'install','sales','wenbin','wb','active',NULL),(11,'董庆庆','15035041655','004',2000.00,0.0100,'1234','2026-08-12','2026-08-14 19:17:21.485','2026-10-29 15:53:49.511','2026-10-03 13:10:25.958',0,'','sales','dongqingqing','dqq','departed','2026-08-20'),(12,'郑玉清','13613431885','005',2000.00,0.0100,'000512','2026-09-02','2026-09-02 11:34:52.090','2026-09-02 11:34:52.090',NULL,0,'','sales','zhengyuqing','zyq','active',NULL),(13,'老板客',NULL,'BOSS001',0.00,0.0000,NULL,'2025-12-01','2026-10-25 14:04:17.921','2026-10-27 14:30:23.000',NULL,1,'','老板','laobanke','lbk','在职',NULL);
/*!40000 ALTER TABLE `employees` ENABLE KEYS */;

--
-- Table structure for table `monthly_attendances`
--

DROP TABLE IF EXISTS `monthly_attendances`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!50503 SET character_set_client = utf8mb4 */;
CREATE TABLE `monthly_attendances` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `employee_id` bigint unsigned DEFAULT NULL,
  `month` varchar(7) COLLATE utf8mb4_general_ci DEFAULT NULL COMMENT '月份 YYYY-MM',
  `leave_days` bigint DEFAULT '0' COMMENT '请假天数',
  `actual_days` bigint DEFAULT '0' COMMENT '实际出勤天数',
  `late_count` bigint DEFAULT '0' COMMENT '迟到次数',
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_monthly_attendances_employee_id` (`employee_id`),
  KEY `idx_monthly_attendances_month` (`month`)
) ENGINE=InnoDB AUTO_INCREMENT=9 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Dumping data for table `monthly_attendances`
--
-- WHERE:  1=1

/*!40000 ALTER TABLE `monthly_attendances` DISABLE KEYS */;
INSERT INTO `monthly_attendances` (`id`, `employee_id`, `month`, `leave_days`, `actual_days`, `late_count`, `created_at`, `updated_at`) VALUES (1,1,'2026-06',4,0,0,'2026-06-11 18:08:24.646','2026-07-03 14:42:53.707'),(2,2,'2026-06',8,0,0,'2026-07-03 14:41:57.592','2026-07-03 14:42:28.510'),(3,1,'2026-07',4,0,0,'2026-07-08 14:39:46.307','2026-08-01 22:17:52.144'),(4,3,'2026-07',4,0,0,'2026-08-01 22:14:55.592','2026-08-01 22:31:27.052'),(5,2,'2026-08',7,0,0,'2026-09-01 22:40:24.908','2026-09-01 22:40:24.918'),(6,11,'2026-08',27,0,0,'2026-09-01 22:40:53.423','2026-09-01 22:41:32.597'),(7,1,'2026-09',4,0,0,'2026-10-03 20:07:58.543','2026-10-03 20:08:03.686'),(8,3,'2026-09',0,0,0,'2026-10-03 20:08:16.549','2026-10-03 20:08:16.569');
/*!40000 ALTER TABLE `monthly_attendances` ENABLE KEYS */;
/*!40103 SET TIME_ZONE=@OLD_TIME_ZONE */;

/*!40101 SET SQL_MODE=@OLD_SQL_MODE */;
/*!40014 SET FOREIGN_KEY_CHECKS=@OLD_FOREIGN_KEY_CHECKS */;
/*!40014 SET UNIQUE_CHECKS=@OLD_UNIQUE_CHECKS */;
/*!40101 SET CHARACTER_SET_CLIENT=@OLD_CHARACTER_SET_CLIENT */;
/*!40101 SET CHARACTER_SET_RESULTS=@OLD_CHARACTER_SET_RESULTS */;
/*!40101 SET COLLATION_CONNECTION=@OLD_COLLATION_CONNECTION */;
/*!40111 SET SQL_NOTES=@OLD_SQL_NOTES */;

-- Dump completed on 2026-10-29 16:00:26
