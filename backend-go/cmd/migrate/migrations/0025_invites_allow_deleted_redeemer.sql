-- Fix for 0022: deleting a user who had redeemed an invite failed outright.
--
-- `used_by_user_id` is ON DELETE SET NULL, so removing the account nulls that
-- column while `used_at` stays set — which the original "both or neither"
-- CHECK rejected, aborting the delete. Account deletion is not optional
-- (bans, GDPR erasure), so the constraint has to give.
--
-- The invariant worth keeping is one-directional: a *claimed* invite always
-- records when it was claimed. The reverse is not true — a spent invite whose
-- redeemer has since been deleted is a perfectly normal state, and it must
-- stay spent rather than quietly becoming reusable.
ALTER TABLE public.invites DROP CONSTRAINT IF EXISTS invites_used_together;

ALTER TABLE public.invites ADD CONSTRAINT invites_used_together CHECK (
  used_by_user_id IS NULL OR used_at IS NOT NULL
);
