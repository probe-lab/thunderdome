output "autoscaling_group_names" {
  value = [for asg in module.autoscaling : asg.autoscaling_group_name]
}
