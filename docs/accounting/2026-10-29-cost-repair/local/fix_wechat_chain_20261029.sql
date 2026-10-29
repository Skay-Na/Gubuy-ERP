START TRANSACTION;;
UPDATE financial_logs SET amount=499.00, remark='订单 202608100001 采购成本修正（零成本库存稀释，按用户2026-10-29确认实价）：¥249.50→¥499.00' WHERE id=646 AND account_id=2 AND amount=249.50;;
COMMIT;